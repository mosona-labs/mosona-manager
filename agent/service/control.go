package service

import (
	"errors"
	"mosona-manager/agent/initsys"
	"os/exec"
	"runtime"
)

const (
	macLaunchdLabel = "cc.mosona.agent"
	macPlistPath    = "/Library/LaunchDaemons/cc.mosona.agent.plist"
)

func linuxServiceCommand(action string) (*exec.Cmd, error) {
	switch init, err := initsys.DetectLinux(); {
	case err != nil:
		return nil, err
	case init == initsys.OpenRC:
		return exec.Command("rc-service", "mosona-agent", action), nil
	default:
		return exec.Command("systemctl", action, "mosona-agent"), nil
	}
}

func Start() ([]byte, error) {
	var cmd *exec.Cmd
	var err error
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
		cmd, err = linuxServiceCommand("start")
	}
	if err != nil {
		return nil, err
	}
	if cmd != nil {
		return cmd.CombinedOutput()
	}
	return nil, errors.New("unsupported operating system")
}

func Stop() ([]byte, error) {
	var cmd *exec.Cmd
	var err error
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("sc", "stop", "mosona-agent")
	case "darwin":
		cmd = exec.Command("launchctl", "bootout", "system/"+macLaunchdLabel)
	case "linux":
		cmd, err = linuxServiceCommand("stop")
	}
	if err != nil {
		return nil, err
	}
	if cmd != nil {
		return cmd.CombinedOutput()
	}
	return nil, errors.New("unsupported operating system")
}

func Restart() ([]byte, error) {
	var cmd *exec.Cmd
	var err error
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
		cmd, err = linuxServiceCommand("restart")
	}
	if err != nil {
		return nil, err
	}
	if cmd != nil {
		return cmd.CombinedOutput()
	}
	return nil, errors.New("unsupported operating system")
}

func Status() ([]byte, error) {
	var cmd *exec.Cmd
	var err error
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("sc", "query", "mosona-agent")
	case "darwin":
		cmd = exec.Command("launchctl", "print", "system/"+macLaunchdLabel)
	case "linux":
		cmd, err = linuxServiceCommand("status")
	}
	if err != nil {
		return nil, err
	}
	if cmd != nil {
		return cmd.CombinedOutput()
	}
	return nil, errors.New("unsupported operating system")
}
