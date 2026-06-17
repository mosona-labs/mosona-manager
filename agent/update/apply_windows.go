//go:build windows

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
	return filepath.Join(agentruntime.InstallDir, "mosona-update-helper.exe")
}

func applyBinary(ctx context.Context, target, downloadURL, wantSHA string) error {
	client := downloadHTTPClient(downloadURL)
	newPath := target + ".new.exe"
	if err := downloadToFile(ctx, client, downloadURL, newPath); err != nil {
		return err
	}
	if err := verifyFileSHA256(newPath, wantSHA); err != nil {
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
	return applyBinary(ctx, target, downloadURL, wantSHA)
}

func RunApplyUpdate() error {
	data, err := os.ReadFile(pendingPath())
	if err != nil {
		return err
	}
	var pending pendingUpdate
	if err := json.Unmarshal(data, &pending); err != nil {
		return err
	}
	if err := verifyFileSHA256(pending.NewPath, pending.WantSHA256); err != nil {
		return err
	}

	if _, err := service.Stop(); err != nil {
		return fmt.Errorf("stop service: %w", err)
	}

	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		_ = os.Remove(pending.TargetPath)
		if err := os.Rename(pending.NewPath, pending.TargetPath); err == nil {
			lastErr = nil
			break
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("replace binary: %w", lastErr)
	}

	_ = os.Remove(pendingPath())
	_ = os.Remove(updateHelperPath())

	if _, err := service.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	return nil
}
