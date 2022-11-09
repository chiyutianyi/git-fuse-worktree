package fs

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
)

type blobNode struct {
	gitNode

	blob *object.Blob

	size uint64
}

func (t *treeFS) newBlobNode(name string, oid plumbing.Hash, mode filemode.FileMode) (*blobNode, error) {
	blob, err := t.repository.BlobObject(oid)
	if err != nil {
		if err == plumbing.ErrObjectNotFound {
			// TODO fetch from remote
			return &blobNode{
				gitNode: gitNode{
					fs:         t,
					name:       name,
					oid:        oid,
					mode:       uint32(mode),
					FileSystem: pathfs.NewDefaultFileSystem(),
					time:       time.Now(),
				},
			}, nil
		}
		return nil, err
	}

	return &blobNode{
		gitNode: gitNode{
			fs:         t,
			name:       name,
			oid:        oid,
			mode:       uint32(mode),
			FileSystem: pathfs.NewDefaultFileSystem(),
			time:       time.Now(),
		},
		blob: blob,
		size: uint64(blob.Size),
	}, nil
}

type memoryFile struct {
	sync.Mutex
	nodefs.File
	blob     *object.Blob
	contents []byte
}

func (f *memoryFile) Read(dest []byte, off int64) (fuse.ReadResult, fuse.Status) {
	f.Lock()
	if f.contents == nil {
		reader, err := f.blob.Reader()
		if err != nil {
			return nil, fuse.EIO
		}
		contents, err := ioutil.ReadAll(reader)
		if err != nil {
			f.Unlock()
			return nil, fuse.EIO
		}
		f.contents = contents
	}
	f.Unlock()
	end := off + int64(len(dest))
	if end > int64(len(f.contents)) {
		end = int64(len(f.contents))
	}
	return fuse.ReadResultData(f.contents[off:end]), fuse.OK
}

func (f *memoryFile) Release() {
	f.Lock()
	defer f.Unlock()
	f.contents = nil
}

func (n *blobNode) LoadMemory() (nodefs.File, error) {
	return &memoryFile{
		File: nodefs.NewDefaultFile(),
		blob: n.blob,
	}, nil
}

type lazyBlobFile struct {
	mu sync.Mutex
	nodefs.File
	ctor func() (nodefs.File, error)
	node *blobNode
}

func (f *lazyBlobFile) SetInode(n *nodefs.Inode) {
}

func (f *lazyBlobFile) Read(dest []byte, off int64) (fuse.ReadResult, fuse.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.File == nil {
		g, err := f.ctor()
		if err != nil {
			log.Printf("opening blob for %s: %v", f.node.oid, err)
			return nil, fuse.EIO
		}
		f.File = g
	}
	return f.File.Read(dest, off)
}

func (f *lazyBlobFile) GetAttr(out *fuse.Attr) fuse.Status {
	*out = *f.node.attr()
	return fuse.OK
}

func (f *lazyBlobFile) Flush() fuse.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.File != nil {
		return f.File.Flush()
	}
	return fuse.OK
}

func (f *lazyBlobFile) Release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.File != nil {
		f.File.Release()
	}
}

func (n *blobNode) LoadDisk() (nodefs.File, error) {
	p := filepath.Join(n.fs.opts.TempDir, n.oid.String())
	if _, err := os.Lstat(p); os.IsNotExist(err) {
		reader, err := n.blob.Reader()
		if err != nil {
			return nil, err
		}

		f, err := os.Create(p)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(f, reader)
		if err != nil {
			return nil, err
		}
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}

	return nodefs.NewLoopbackFile(f), nil
}

func (n *blobNode) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}

	ctor := n.LoadMemory
	if n.fs.opts.Disk {
		ctor = n.LoadDisk
	}

	if !n.fs.opts.Lazy {
		f, err := ctor()
		if err != nil {
			return nil, fuse.ToStatus(err)
		}
		return f, fuse.OK
	}

	return &lazyBlobFile{
		ctor: ctor,
		node: n,
	}, fuse.OK
}

func (n *blobNode) attr() *fuse.Attr {
	return &fuse.Attr{Mode: n.mode, Size: n.size, Ino: n.Ino(), Mtime: uint64(n.time.Unix()), Atime: uint64(n.time.Unix()), Ctime: uint64(n.time.Unix())}
}

func (n *blobNode) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	return n.attr(), fuse.OK
}

type mockBlobNode struct {
	gitNode

	contents []byte
}

func (t *treeFS) newMockBlobNode(name string, contents []byte) (*mockBlobNode, error) {
	return &mockBlobNode{
		gitNode: gitNode{
			fs:         t,
			name:       name,
			mode:       uint32(fuse.S_IFREG),
			FileSystem: pathfs.NewDefaultFileSystem(),
			time:       time.Now(),
		},
		contents: contents,
	}, nil
}

func (n *mockBlobNode) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	return &fuse.Attr{Mode: n.mode, Size: uint64(len(n.contents)), Ino: n.Ino(), Mtime: uint64(n.time.Unix()), Atime: uint64(n.time.Unix()), Ctime: uint64(n.time.Unix())}, fuse.OK
}

func (n *mockBlobNode) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}

	return &memoryFile{
		File:     nodefs.NewDefaultFile(),
		contents: n.contents,
	}, fuse.OK
}
