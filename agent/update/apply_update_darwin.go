//go:build darwin

package update

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const (
	macLaunchdLabel = "cc.mosona.agent"
	macPlistPath    = "/Library/LaunchDaemons/cc.mosona.agent.plist"
)

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

	_ = exec.Command("launchctl", "bootout", "system/"+macLaunchdLabel).Run()
	time.Sleep(2 * time.Second)

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

	if output, err := exec.Command("launchctl", "bootstrap", "system", macPlistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("bootstrap service: %w (%s)", err, string(output))
	}
	if output, err := exec.Command("launchctl", "kickstart", "-k", "system/"+macLaunchdLabel).CombinedOutput(); err != nil {
		return fmt.Errorf("kickstart service: %w (%s)", err, string(output))
	}
	return nil
}
