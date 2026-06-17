package update

import (
	agentruntime "mosona-manager/agent/runtime"
	"os"
	"path/filepath"
	"runtime"
)

func installedBinaryPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(agentruntime.InstallDir, "mosona-agent.exe")
	}
	return filepath.Join(agentruntime.InstallDir, "mosona-agent")
}

func resolveTargetPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", err
	}
	installed := installedBinaryPath()
	if _, err := os.Stat(installed); err == nil {
		return installed, nil
	}
	return execPath, nil
}
