package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoinUnderRoot(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "static", "index.html")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := SafeJoinUnderRoot(root, "/static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatal(err)
	}

	if _, err := SafeJoinUnderRoot(root, "/../../../etc/passwd"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}