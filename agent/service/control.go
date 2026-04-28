package service

import (
	"errors"
	"os/exec"
	"runtime"
)

const (
	macLaunchdLabel = "cc.mosona.agent"
	macPlistPath    = "/Library/LaunchDaemons/cc.mosona.agent.plist"
)

func Start() ([]byte, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("sc", "start", "mosona-agent")
	case "darwin":
		if err := exec.Command("launchctl", "print", "system/"+macLaunchdLabel).Run(); err != nil {
			if output, err := exec.Command("launchctl", "bootstrap", "system", macPlistPath).CombinedOutput(); err != nil {
				return output, err
			}
		}
		cmd = exec.Command("launchctl", "kickstart", "-k", "system/"+macLaunchdLabel)
	case "linux":
		cmd = exec.Command("systemctl", "start", "mosona-agent")
	}
	if cmd != nil {
		return cmd.CombinedOutput()
	}
	return nil, errors.New("unsupported operating system")
}

func Stop() ([]byte, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("sc", "stop", "mosona-agent")
	case "darwin":
		cmd = exec.Command("launchctl", "bootout", "system/"+macLaunchdLabel)
	case "linux":
		cmd = exec.Command("systemctl", "stop", "mosona-agent")
	}
	if cmd != nil {
		return cmd.CombinedOutput()
	}
	return nil, errors.New("unsupported operating system")
}

func Restart() ([]byte, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("sc", "stop", "mosona-agent")
		if output, err := cmd.CombinedOutput(); err != nil {
			return output, err
		}
		cmd = exec.Command("sc", "start", "mosona-agent")
	case "darwin":
		_ = exec.Command("launchctl", "bootout", "system/"+macLaunchdLabel).Run()
		if output, err := exec.Command("launchctl", "bootstrap", "system", macPlistPath).CombinedOutput(); err != nil {
			return output, err
		}
		cmd = exec.Command("launchctl", "kickstart", "-k", "system/"+macLaunchdLabel)
	case "linux":
		cmd = exec.Command("systemctl", "restart", "mosona-agent")
	}
	if cmd != nil {
		return cmd.CombinedOutput()
	}
	return nil, errors.New("unsupported operating system")
}

func Status() ([]byte, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("sc", "query", "mosona-agent")
	case "darwin":
		cmd = exec.Command("launchctl", "print", "system/"+macLaunchdLabel)
	case "linux":
		cmd = exec.Command("systemctl", "status", "mosona-agent")
	}
	if cmd != nil {
		return cmd.CombinedOutput()
	}
	return nil, errors.New("unsupported operating system")
}
