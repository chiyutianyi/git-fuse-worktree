package fs

import (
	"fmt"
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
}

func (t *treeFS) newDirNode(name string, oid plumbing.Hash) *dirNode {
	return &dirNode{
		gitNode: gitNode{
			FileSystem: pathfs.NewDefaultFileSystem(),
			fs:         t,
			name:       name,
			oid:        oid,
			mode:       fuse.S_IFDIR | 0755,
			time:       time.Now(),
		},
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
		n.childrenMap = make(map[string]gitEntry, len(n.tree.Entries))
		for _, entry := range n.tree.Entries {
			isdir := entry.Mode&syscall.S_IFDIR != 0
			if isdir {
				chNode = n.fs.newDirNode(entry.Name, entry.Hash)
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
