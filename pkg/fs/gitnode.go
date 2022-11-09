package fs

import (
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
)

type gitEntry interface {
	// Mode is the file's mode. Only the high bits (eg. S_IFDIR)
	// are considered.
	Mode() uint32

	// Name is the basename of the file in the directory.
	Name() string

	// Ino is the inode number.
	Ino() uint64

	GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status)
}

type gitNode struct {
	pathfs.FileSystem

	fs *treeFS

	inode uint64
	name  string
	mode  uint32
	oid   plumbing.Hash

	time time.Time
}

func (n *gitNode) String() string {
	return "gitfs"
}

func (n *gitNode) Mode() uint32 {
	return n.mode
}

func (n *gitNode) Name() string {
	return n.name
}

func (n *gitNode) Ino() uint64 {
	if n.inode > 0 {
		return n.inode
	}
	n.inode = n.fs.geninodeid()
	return n.inode
}

func (n *gitNode) Access(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.OK
}

func (n *gitNode) GetXAttr(name string, attribute string, context *fuse.Context) (data []byte, code fuse.Status) {
	if attribute == "unix_digest_hash_attribute_name" {
		return []byte(n.oid.String()), fuse.OK
	}
	return nil, fuse.ENODATA
}

func (n *gitNode) ListXAttr(name string, context *fuse.Context) (attributes []string, code fuse.Status) {
	return []string{"unix_digest_hash_attribute_name"}, fuse.OK
}
