//go:build darwin

package update

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	agentruntime "mosona-manager/agent/runtime"
	"mosona-manager/agent/service"
)

func updateHelperPath() string {
	return filepath.Join(agentruntime.InstallDir, "mosona-update-helper")
}

func applyBinary(ctx context.Context, target, downloadURL, wantSHA string) error {
	client := newGitHubClient()
	newPath := target + ".new"
	if err := downloadToFile(ctx, client, downloadURL, newPath); err != nil {
		return err
	}
	if err := verifyFileSHA256(newPath, wantSHA); err != nil {
		_ = os.Remove(newPath)
		return err
	}
	if err := os.Chmod(newPath, 0755); err != nil {
		_ = os.Remove(newPath)
		return err
	}

	pending := pendingUpdate{
		TargetPath: target,
		NewPath:    newPath,
		WantSHA256: wantSHA,
	}
	data, err := json.Marshal(pending)
	if err != nil {
		_ = os.Remove(newPath)
		return err
	}
	if err := os.MkdirAll(agentruntime.InstallDir, 0755); err != nil {
		_ = os.Remove(newPath)
		return err
	}
	if err := os.WriteFile(pendingPath(), data, 0600); err != nil {
		_ = os.Remove(newPath)
		return err
	}

	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return err
	}

	helper := updateHelperPath()
	if err := copyFile(execPath, helper); err != nil {
		return fmt.Errorf("stage update helper: %w", err)
	}
	if err := os.Chmod(helper, 0755); err != nil {
		return err
	}

	cmd := exec.Command(helper, "apply-update")
	cmd.Dir = agentruntime.InstallDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		time.Sleep(3 * time.Second)
		_, _ = service.Stop()
		os.Exit(0)
	}()
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
