//go:build linux

package update

import "fmt"

func RunApplyUpdate() error {
	return fmt.Errorf("apply-update is only used on Windows and macOS")
}
