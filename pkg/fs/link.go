package fs

import (
	"io/ioutil"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
	log "github.com/sirupsen/logrus"
)

type linkNode struct {
	sync.Mutex
	gitNode
	target []byte
}

func (t *treeFS) newLinkNode(name string, oid plumbing.Hash) *linkNode {
	return &linkNode{
		gitNode: gitNode{
			fs:         t,
			name:       name,
			oid:        oid,
			FileSystem: pathfs.NewDefaultFileSystem(),
			time:       time.Now(),
		},
	}
}

func (n *linkNode) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	return &fuse.Attr{Mode: fuse.S_IFLNK | 0755, Ino: n.Ino(), Mtime: uint64(n.time.Unix()), Atime: uint64(n.time.Unix()), Ctime: uint64(n.time.Unix())}, fuse.OK
}

func (n *linkNode) Readlink(name string, context *fuse.Context) (string, fuse.Status) {
	if n.target != nil {
		return string(n.target), fuse.OK
	}
	n.Lock()
	defer n.Unlock()
	blob, err := n.fs.repository.BlobObject(n.oid)
	if err != nil {
		return "", fuse.EIO
	}

	reader, err := blob.Reader()
	if err != nil {
		log.Errorf("Error reading blob %s: %s", n.oid.String(), err)
		return "", fuse.EIO
	}
	content, err := ioutil.ReadAll(reader)
	if err != nil {
		log.Errorf("Error reading blob %s: %s", n.oid.String(), err)
		return "", fuse.EIO
	}

	n.target = append([]byte{}, content...)
	return string(n.target), fuse.OK
}
