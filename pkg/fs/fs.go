package fs

import (
	"fmt"
	"sync/atomic"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
)

type GitFSOptions struct {
	Lazy    bool
	Disk    bool
	TempDir string
}

type treeFS struct {
	repository *gogit.Repository

	opts *GitFSOptions

	automaticIno uint64
}

func NewTreeFSRoot(path, revision string, opts *GitFSOptions) (pathfs.FileSystem, error) {
	repository, err := gogit.PlainOpen(path)
	if err != nil {
		return nil, err
	}
	t := treeFS{
		repository:   repository,
		opts:         opts,
		automaticIno: 0,
	}
	oid, err := repository.ResolveRevision(plumbing.Revision(revision))
	if err != nil {
		return nil, fmt.Errorf("resolve revision: %v", err)
	}
	commit, err := repository.CommitObject(*oid)
	if err != nil {
		return nil, fmt.Errorf("commit object: %v", err)
	}
	root, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("root tree: %v", err)
	}
	return t.newDirNode("", root.Hash), nil
}

func (t *treeFS) onMount(nodeFs *pathfs.PathNodeFs) {
}

func (t *treeFS) geninodeid() uint64 {
	return atomic.AddUint64(&t.automaticIno, 1)
}
