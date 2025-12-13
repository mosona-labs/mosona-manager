package runtime

import (
	"runtime"
)

var InstallDir = defaultInstallDir()

func defaultInstallDir() string {
	if runtime.GOOS == "windows" {
		return "C:\\Program Files\\Mosona Agent"
	}
	return "/opt/mosona-agent"
}
