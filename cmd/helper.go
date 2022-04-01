package main

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

func getMountpoint(gitdir, worktree string) string {
	return fmt.Sprintf("%s/%s", gitdir, worktree)
}

func getWorktree(gitdir, worktree string) string {
	return fmt.Sprintf("%s/worktrees/%s", gitdir, worktree)
}

func bindGitDir(flags *pflag.FlagSet, gitdir *string) {
	flags.StringVarP(gitdir, "git-dir", "C", "", "git dir")
}

func getGitDir(gitDir string) string {
	if gitDir != "" && !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(os.Getenv("PWD"), gitDir)
	}
	if gitDir == "" {
		log.Fatalf("git-dir not set")
	}
	return gitDir
}
