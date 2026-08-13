//go:build unix

package config

import (
	"fmt"
	"os"
	"syscall"
)

func openFileNoFollow(path string, flag int, mode os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flag|syscall.O_NOFOLLOW, mode)
}

func validateOwner(path string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot determine owner of %s", path)
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("%s is owned by uid %d, expected uid %d", path, stat.Uid, os.Geteuid())
	}
	return nil
}
