//go:build unix

package encrypt

import (
	"errors"
	"syscall"
	"testing"
)

func TestIgnoreUnsupportedDirectorySync(t *testing.T) {
	for _, err := range []error{syscall.EINVAL, syscall.ENOTSUP} {
		if !ignoreUnsupportedDirectorySync(err) {
			t.Fatalf("error %v was not treated as an unsupported directory sync", err)
		}
	}
	if ignoreUnsupportedDirectorySync(errors.New("I/O failure")) {
		t.Fatal("unexpected directory sync error was ignored")
	}
}
