package install

import (
	"io"
	agentruntime "mosona-manager/agent/runtime"
	"os"
	"path"
	"runtime"
)

func copyBinaryToInstallDir() error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	var destPath string
	if runtime.GOOS == "windows" {
		destPath = path.Join(agentruntime.InstallDir, "mosona-agent.exe")
	} else {
		destPath = path.Join(agentruntime.InstallDir, "mosona-agent")
	}

	sourceFile, err := os.Open(execPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = sourceFile.Close()
	}()

	destFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = destFile.Close()
	}()

	if _, err = io.Copy(destFile, sourceFile); err != nil {
		return err
	}
	if err = os.Chmod(destPath, 0755); err != nil {
		return err
	}

	return nil
}
