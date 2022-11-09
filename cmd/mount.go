package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/chiyutianyi/git-fuse-worktree/pkg/fs"
)

type gitfsCmd struct {
	o struct {
		debug    bool
		logLevel string

		gitDir string

		lazy    bool
		disk    bool
		tempDir string

		portable        bool
		entryTtl        float64
		negativeTtl     float64
		delcacheTtl     float64
		branchcacheTtl  float64
		deletionDirname string
	}
}

func (cmd *gitfsCmd) getLogLevel() (logLevel log.Level) {
	logLevel, err := log.ParseLevel(strings.ToLower(cmd.o.logLevel))
	if err != nil {
		return log.InfoLevel
	}
	return logLevel
}

func (cmd *gitfsCmd) Run(_ *cobra.Command, args []string) {
	log.SetLevel(cmd.getLogLevel())
	if len(args) < 2 {
		log.Fatalf("usage: %s MOUNT", os.Args[0])
	}

	revision := args[1]

	mp := args[0]
	worktree := fmt.Sprintf("%s/worktrees/%s", cmd.o.gitDir, args[0])

	doCheckAndUnmount(mp)

	tempDir, err := ioutil.TempDir("", cmd.o.tempDir)
	if err != nil {
		log.Fatalf("TempDir: %v", err)
	}

	opts := &fs.GitFSOptions{
		Lazy:    cmd.o.lazy,
		Disk:    cmd.o.disk,
		TempDir: tempDir,
	}

	root, err := fs.NewTreeFSRoot(cmd.o.gitDir, revision, worktree, opts)
	if err != nil {
		log.Fatalf("NewTreeFSRoot: %v", err)
	}

	nodeFs := pathfs.NewPathNodeFs(root, &pathfs.PathNodeFsOptions{ClientInodes: true})
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

	signal.Ignore(syscall.SIGPIPE)
	signalChan := make(chan os.Signal, 10)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		for {
			<-signalChan
			go func() { doUmount(mp, true) }()
			go func() {
				time.Sleep(time.Second * 3)
				os.Exit(1)
			}()
		}
	}()

	mountState, _, err := nodefs.MountRoot(mp, nodeFs.Root(), &mOpts)
	if err != nil {
		log.Fatal("Mount fail:", err)
	}

	mountState.Serve()
}

func init() {
	gitfs := &gitfsCmd{}

	cmd := &cobra.Command{
		Use:   "mount",
		Short: "mount gitfs",
		Run:   gitfs.Run,
	}
	Cmd.AddCommand(cmd)

	flags := cmd.Flags()
	flags.BoolVarP(&gitfs.o.debug, "debug", "d", false, "debug")
	flags.StringVarP(&gitfs.o.logLevel, "log-level", "", "info", "log level")
	flags.StringVarP(&gitfs.o.gitDir, "git-dir", "", "", "git dir")

	flags.BoolVarP(&gitfs.o.lazy, "lazy", "", true, "only read contents for reads")
	flags.BoolVarP(&gitfs.o.disk, "disk", "", false, "don't use intermediate files")
	flags.StringVarP(&gitfs.o.tempDir, "tempdir", "", "gitfs", "tempdir name")

	flags.BoolVarP(&gitfs.o.portable, "portable", "", false, "use 32 bit inodes")
	flags.Float64VarP(&gitfs.o.entryTtl, "entry-ttl", "", 1.0, "fuse entry cache TTL.")
	flags.Float64VarP(&gitfs.o.negativeTtl, "negative-ttl", "", 1.0, "fuse negative entry cache TTL.")
	flags.Float64VarP(&gitfs.o.delcacheTtl, "delcache-cache-ttl", "", 5.0, "Deletion cache TTL in seconds.")
	flags.Float64VarP(&gitfs.o.branchcacheTtl, "branchcache-ttl", "", 5.0, "Branch cache TTL in seconds.")
	flags.StringVarP(&gitfs.o.deletionDirname, "deletion-dirname", "", "GOUNIONFS_DELETIONS", "Directory name to use for deletions.")
}
