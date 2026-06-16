package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func SafeJoinUnderRoot(root, requestPath string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	cleaned := filepath.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	if cleaned == "/" || cleaned == "." {
		return root, nil
	}
	rel := strings.TrimPrefix(cleaned, "/")
	target := filepath.Join(root, rel)
	target, err = filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rootPrefix := root + string(os.PathSeparator)
	if target != root && !strings.HasPrefix(target, rootPrefix) {
		return "", fmt.Errorf("path escapes root")
	}
	return target, nil
}