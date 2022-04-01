package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type removeCmd struct {
	o struct {
		debug bool

		force bool

		gitDir string
	}
}

func (cmd *removeCmd) Run(_ *cobra.Command, args []string) {
	if len(args) < 1 {
		log.Fatalf("usage: %s remove <worktree>", os.Args[0])
	}

	gitDir := getGitDir(cmd.o.gitDir)

	mp := getMountpoint(gitDir, args[0])
	worktree := getWorktree(gitDir, args[0])

	if err := doUmount(mp, cmd.o.force); err != nil {
		if cmd.o.force {
			log.Warnf("unmount %s error: %v", mp, err)
		} else {
			log.Fatalf("unmount %s error: %v", mp, err)
		}
	}
	if err := os.Remove(worktree); err != nil {
		if cmd.o.force {
			log.Warnf("remove %s error: %v", worktree, err)
		} else {
			log.Fatalf("remove %s error: %v", worktree, err)
		}
	}
}

func init() {
	remove := &removeCmd{}

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "remove <worktree>",
		Run:   remove.Run,
	}
	Cmd.AddCommand(cmd)

	flags := cmd.Flags()
	flags.BoolVarP(&remove.o.debug, "debug", "d", false, "debug")
	flags.BoolVarP(&remove.o.force, "force", "f", false, "force")
	bindGitDir(flags, &remove.o.gitDir)
}
