package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func SafeJoinUnderRoot(root, requestPath string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if containsParentPathSegment(requestPath) {
		return "", fmt.Errorf("path escapes root")
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

func containsParentPathSegment(path string) bool {
	return slices.Contains(strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}), "..")
}
