package install

import (
	"fmt"
	"mosona-manager/agent/runtime"
	"os"
)

func prepareInstallDir() error {
	if _, err := os.Lstat(runtime.InstallDir); os.IsNotExist(err) {
		return os.MkdirAll(runtime.InstallDir, 0o700)
	} else if err != nil {
		return err
	}

	fmt.Print("Do you want to reinstall agent? (y/N): ")
	var response string
	_, _ = fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		return fmt.Errorf("installation cancelled by user")
	}

	_ = UninstallService()
	if err := os.RemoveAll(runtime.InstallDir); err != nil {
		return err
	}
	return os.MkdirAll(runtime.InstallDir, 0o700)
}

func installDir() string {
	return runtime.InstallDir
}
