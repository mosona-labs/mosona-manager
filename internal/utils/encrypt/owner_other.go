//go:build !unix

package encrypt

import "os"

func openKeyFile(path string) (*os.File, error) {
	return os.Open(path)
}

func validateOwner(string, os.FileInfo) error {
	return nil
}

func ignoreUnsupportedDirectorySync(error) bool {
	return false
}
