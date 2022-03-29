package main

import (
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
	"github.com/hanwen/go-fuse/v2/unionfs"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/chiyutianyi/git-fuse-worktree/pkg/fs"
)

type addCmd struct {
	o struct {
		debug           bool
		logLevel        string
		lazy            bool
		disk            bool
		tempDir         string
		portable        bool
		entryTtl        float64
		negativeTtl     float64
		delcacheTtl     float64
		branchcacheTtl  float64
		deletionDirname string
	}
}

func (cmd *addCmd) getLogLevel() (logLevel log.Level) {
	logLevel, err := log.ParseLevel(strings.ToLower(cmd.o.logLevel))
	if err != nil {
		return log.InfoLevel
	}
	return logLevel
}

func (cmd *addCmd) Run(_ *cobra.Command, args []string) {
	log.SetLevel(cmd.getLogLevel())
	if len(args) < 3 {
		log.Fatalf("usage: %s MOUNT", os.Args[0])
	}
	mp := args[0]
	upper := args[1]
	gitRepo := args[2]

	doCheckAndUnmount(mp)

	tempDir, err := ioutil.TempDir("", cmd.o.tempDir)
	if err != nil {
		log.Fatalf("TempDir: %v", err)
	}

	components := strings.Split(gitRepo, ":")
	if len(components) != 2 {
		log.Fatalf("must have 2 components: %q", gitRepo)
	}

	opts := &fs.GitFSOptions{
		Lazy:    cmd.o.lazy,
		Disk:    cmd.o.disk,
		TempDir: tempDir,
	}

	ufsOptions := unionfs.UnionFsOptions{
		DeletionCacheTTL: time.Duration(cmd.o.delcacheTtl * float64(time.Second)),
		BranchCacheTTL:   time.Duration(cmd.o.branchcacheTtl * float64(time.Second)),
		DeletionDirName:  cmd.o.deletionDirname,
	}

	fses := make([]pathfs.FileSystem, 0)
	fses = append(fses, pathfs.NewLoopbackFileSystem(upper))

	root, err := fs.NewTreeFSRoot(components[0], components[1], opts)
	if err != nil {
		log.Fatalf("NewTreeFSRoot: %v", err)
	}

	fses = append(fses, root)
	ufs, err := unionfs.NewUnionFs(fses, ufsOptions)
	if err != nil {
		log.Fatalf("NewUnionFs: %v", err)
	}

	nodeFs := pathfs.NewPathNodeFs(ufs, &pathfs.PathNodeFsOptions{ClientInodes: true})
	mOpts := nodefs.Options{
		EntryTimeout:    time.Duration(cmd.o.entryTtl * float64(time.Second)),
		AttrTimeout:     time.Duration(cmd.o.entryTtl * float64(time.Second)),
		NegativeTimeout: time.Duration(cmd.o.negativeTtl * float64(time.Second)),
		PortableInodes:  cmd.o.portable,
		Owner: &fuse.Owner{
			Uid: uint32(os.Getuid()),
			Gid: uint32(os.Getgid()),
		},
		Debug: cmd.o.debug,
	}

	mountState, _, err := nodefs.MountRoot(mp, nodeFs.Root(), &mOpts)
	if err != nil {
		log.Fatal("Mount fail:", err)
	}

	mountState.Serve()
}

func init() {
	add := &addCmd{}

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create <path> and checkout <commit-ish> into it",
		Run:   add.Run,
	}
	Cmd.AddCommand(cmd)

	flags := cmd.Flags()
	flags.BoolVarP(&add.o.debug, "debug", "d", false, "debug")
	flags.StringVarP(&add.o.logLevel, "log-level", "", "info", "log level")

	flags.BoolVarP(&add.o.lazy, "lazy", "", true, "only read contents for reads")
	flags.BoolVarP(&add.o.disk, "disk", "", false, "don't use intermediate files")
	flags.StringVarP(&add.o.tempDir, "tempdir", "", "gitfs", "tempdir name")

	flags.BoolVarP(&add.o.portable, "portable", "", false, "use 32 bit inodes")
	flags.Float64VarP(&add.o.entryTtl, "entry-ttl", "", 1.0, "fuse entry cache TTL.")
	flags.Float64VarP(&add.o.negativeTtl, "negative-ttl", "", 1.0, "fuse negative entry cache TTL.")
	flags.Float64VarP(&add.o.delcacheTtl, "delcache-cache-ttl", "", 5.0, "Deletion cache TTL in seconds.")
	flags.Float64VarP(&add.o.branchcacheTtl, "branchcache-ttl", "", 5.0, "Branch cache TTL in seconds.")
	flags.StringVarP(&add.o.deletionDirname, "deletion-dirname", "", "GOUNIONFS_DELETIONS", "Directory name to use for deletions.")
}
