//go:build linux

package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"mosona-manager/agent/service"
)

func applyBinary(ctx context.Context, target, downloadURL, wantSHA string) error {
	client := newGitHubClient()
	tmp := target + ".new"
	if err := downloadToFile(ctx, client, downloadURL, tmp); err != nil {
		return err
	}
	if err := verifyFileSHA256(tmp, wantSHA); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0755); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if _, err := service.Restart(); err != nil {
		return fmt.Errorf("restart service: %w", err)
	}
	return nil
}

func stageAndApply(ctx context.Context, downloadURL, wantSHA string) error {
	target, err := resolveTargetPath()
	if err != nil {
		return err
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	return applyBinary(ctx, target, downloadURL, wantSHA)
}
