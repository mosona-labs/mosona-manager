package utils

import (
	"path/filepath"
	"testing"
)

func TestSafeJoinUnderRoot(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "static", "index.html")

	tests := []struct {
		name        string
		requestPath string
		want        string
		wantErr     bool
	}{
		{name: "file", requestPath: "/static/index.html", want: nested},
		{name: "root", requestPath: "/", want: root},
		{name: "dot segment", requestPath: "/static/./index.html", want: nested},
		{name: "dot prefix is a filename", requestPath: "/static/..index.html", want: filepath.Join(root, "static", "..index.html")},
		{name: "leading traversal", requestPath: "/../../../etc/passwd", wantErr: true},
		{name: "relative traversal", requestPath: "../etc/passwd", wantErr: true},
		{name: "nested traversal", requestPath: "/static/../index.html", wantErr: true},
		{name: "backslash traversal", requestPath: `\static\..\index.html`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := SafeJoinUnderRoot(root, test.requestPath)
			if test.wantErr {
				if err == nil {
					t.Fatalf("SafeJoinUnderRoot(%q) = %q, want an error", test.requestPath, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SafeJoinUnderRoot(%q): %v", test.requestPath, err)
			}
			if got != test.want {
				t.Fatalf("SafeJoinUnderRoot(%q) = %q, want %q", test.requestPath, got, test.want)
			}
		})
	}
}
