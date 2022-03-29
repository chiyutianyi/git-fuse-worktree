package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	log "github.com/sirupsen/logrus"
)

func doCheckAndUnmount(mp string) {
	fi, err := os.Stat(mp)
	if !strings.Contains(mp, ":") && err != nil {
		if err := os.MkdirAll(mp, 0777); err != nil {
			if os.IsExist(err) {
				// a broken mount point, umount it
				if err = doUmount(mp, true); err != nil {
					log.Fatalf("umount %s: %s", mp, err)
				}
			} else {
				log.Fatalf("create %s: %s", mp, err)
			}
		}
	} else if err == nil && fi.Size() == 0 {
		// a broken mount point, umount it
		if err = doUmount(mp, true); err != nil {
			log.Fatalf("umount %s: %s", mp, err)
		}
	}
}

func doUmount(mp string, force bool) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if force {
			cmd = exec.Command("diskutil", "umount", "force", mp)
		} else {
			cmd = exec.Command("diskutil", "umount", mp)
		}
	case "linux":
		if _, err := exec.LookPath("fusermount"); err == nil {
			if force {
				cmd = exec.Command("fusermount", "-uz", mp)
			} else {
				cmd = exec.Command("fusermount", "-u", mp)
			}
		} else {
			if force {
				cmd = exec.Command("umount", "-l", mp)
			} else {
				cmd = exec.Command("umount", mp)
			}
		}
	default:
		return fmt.Errorf("OS %s is not supported", runtime.GOOS)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Print(string(out))
	}
	return err
}
