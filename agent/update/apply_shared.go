package update

import (
	"io"
	"os"
	"path/filepath"

	agentruntime "mosona-manager/agent/runtime"
)

type pendingUpdate struct {
	TargetPath string `json:"target_path"`
	NewPath    string `json:"new_path"`
	WantSHA256 string `json:"want_sha256"`
}

func pendingPath() string {
	return filepath.Join(agentruntime.InstallDir, "update.pending.json")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
