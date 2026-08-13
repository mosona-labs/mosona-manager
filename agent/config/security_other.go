//go:build !unix

package config

import "os"

func openFileNoFollow(path string, flag int, mode os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flag, mode)
}

func validateOwner(string, os.FileInfo) error {
	return nil
}
