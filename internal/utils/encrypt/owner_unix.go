//go:build unix

package encrypt

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func openKeyFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
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

func ignoreUnsupportedDirectorySync(err error) bool {
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP)
}
