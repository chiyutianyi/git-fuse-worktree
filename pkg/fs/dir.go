package fs

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
	log "github.com/sirupsen/logrus"
)

type dirNode struct {
	sync.Mutex
	gitNode

	tree        *object.Tree
	children    []gitEntry
	childrenMap map[string]gitEntry

	parents []fuse.DirEntry
}

func (t *treeFS) newDirNode(gitdir, worktree, name string, oid plumbing.Hash) *dirNode {
	var (
		parents     []fuse.DirEntry
		ino         uint64
		mode        uint32 = fuse.S_IFDIR | 0755
		children    []gitEntry
		childrenMap = map[string]gitEntry{}
	)
	if gitdir != "" {
		st := syscall.Stat_t{}
		err := syscall.Lstat(gitdir, &st)
		if err != nil {
			log.Errorf("get root %s stat: %s", gitdir, err)
		} else {
			ino = st.Ino
			mode = uint32(st.Mode)
			parents = append(parents, fuse.DirEntry{mode, ".", ino})
			dir := filepath.Dir(gitdir)
			err = syscall.Lstat(dir, &st)
			if err != nil {
				log.Errorf("get root %s stat: %s", dir, err)
			} else {
				parents = append(parents, fuse.DirEntry{uint32(st.Mode), "..", st.Ino})
			}
		}
		gitRoot, err := t.newMockBlobNode(".git", []byte(fmt.Sprintf("gitdir: %s", worktree)))
		if err != nil {
			log.Errorf("newMockBlobNode .git: %s", err)
		} else {
			children = append(children, gitRoot)
			childrenMap[".git"] = gitRoot
		}
	}
	return &dirNode{
		gitNode: gitNode{
			FileSystem: pathfs.NewDefaultFileSystem(),
			fs:         t,
			inode:      ino,
			name:       name,
			oid:        oid,
			mode:       mode,
			time:       time.Now(),
		},
		parents:     parents,
		children:    children,
		childrenMap: childrenMap,
	}
}

func (n *dirNode) OnMount(nodeFs *pathfs.PathNodeFs) {
	n.fs.onMount(nodeFs)
}

// Directory handling
func (n *dirNode) getChildren() fuse.Status {
	n.Lock()
	defer n.Unlock()
	if n.tree == nil {
		tree, err := n.fs.repository.TreeObject(n.oid)
		if err != nil {
			return fuse.ENOENT
		}
		n.tree = tree
		var chNode gitEntry
		for _, entry := range n.tree.Entries {
			isdir := entry.Mode&syscall.S_IFDIR != 0
			if isdir {
				chNode = n.fs.newDirNode("", "", entry.Name, entry.Hash)
			} else if entry.Mode&^07777 == syscall.S_IFLNK {
				chNode = n.fs.newLinkNode(entry.Name, entry.Hash)
			} else if entry.Mode&^07777 == syscall.S_IFREG {
				chNode, err = n.fs.newBlobNode(entry.Name, entry.Hash, entry.Mode)
				if err != nil {
					panic(fmt.Sprintf("newBlobNode %s: %s", entry.Name, err))
				}
			} else {
				panic(fmt.Sprintf("unexpected file %06o for %s\n", entry.Mode, entry.Hash))
			}
			n.children = append(n.children, chNode)
			n.childrenMap[entry.Name] = chNode
		}
	}
	return fuse.OK
}

// Directory handling
func (n *dirNode) OpenDir(name string, context *fuse.Context) (stream []fuse.DirEntry, code fuse.Status) {
	defer func() {
		log.Debugf("OpenDir current <%s> name <%s>: chilren: %v, status: %v", n.oid, name, len(stream), code)
	}()
	if n.tree == nil {
		if code = n.getChildren(); code != fuse.OK {
			return
		}
	}

	if name == "" {
		stream = append(stream, n.parents...)
		for _, ch := range n.children {
			stream = append(stream, fuse.DirEntry{ch.Mode(), ch.Name(), ch.Ino()})
		}
		code = fuse.OK
		return
	}

	rs := strings.SplitN(name, "/", 2)
	child, ok := n.childrenMap[rs[0]]
	if !ok {
		code = fuse.ENOENT
		return
	}
	if child.Mode()&fuse.S_IFDIR == 0 {
		code = fuse.ENOENT
		return
	}
	if len(rs) == 1 {
		return child.(*dirNode).OpenDir("", context)
	}
	return child.(*dirNode).OpenDir(rs[1], context)
}

func (n *dirNode) GetAttr(name string, context *fuse.Context) (attr *fuse.Attr, code fuse.Status) {
	defer func() {
		log.Debugf("Getattr current <%s> name <%s> attr: %v, status: %v", n.oid, name, attr, code)
	}()
	if name == "" {
		attr = &fuse.Attr{Mode: n.mode, Size: 64, Ino: n.Ino(), Mtime: uint64(n.time.Unix()), Atime: uint64(n.time.Unix()), Ctime: uint64(n.time.Unix())}
		code = fuse.OK
		return
	}

	if n.tree == nil {
		if code = n.getChildren(); code != fuse.OK {
			return
		}
	}

	rs := strings.SplitN(name, "/", 2)
	child, ok := n.childrenMap[rs[0]]
	if !ok {
		return nil, fuse.ENOENT
	}
	if len(rs) == 1 {
		return child.GetAttr("", context)
	}
	return child.GetAttr(rs[1], context)
}

func (n *dirNode) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	defer func() {
		log.Debugf("Open current <%s> name <%s>, status: %v", n.oid, name, code)
	}()
	if n.tree == nil {
		if code = n.getChildren(); code != fuse.OK {
			return nil, code
		}
	}

	rs := strings.SplitN(name, "/", 2)
	child, ok := n.childrenMap[rs[0]]
	if !ok {
		return nil, fuse.ENOENT
	}
	if len(rs) == 1 {
		if len(n.parents) > 0 && name == ".git" {
			return child.(*mockBlobNode).Open(name, flags, context)
		}
		return child.(*blobNode).Open(name, flags, context)
	}
	return child.(*dirNode).Open(rs[1], flags, context)
}

func (n *dirNode) Readlink(name string, context *fuse.Context) (link string, code fuse.Status) {
	defer func() {
		log.Debugf("Readlink current <%s> name <%s> link: %v, status: %v", n.oid, name, link, code)
	}()
	if n.tree == nil {
		if code = n.getChildren(); code != fuse.OK {
			return "", code
		}
	}

	rs := strings.SplitN(name, "/", 2)
	child, ok := n.childrenMap[rs[0]]
	if !ok {
		return "", fuse.ENOENT
	}
	if len(rs) == 1 {
		return child.(*linkNode).Readlink("", context)
	}
	return child.(*dirNode).Readlink(rs[1], context)
}
