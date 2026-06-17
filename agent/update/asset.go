package update

import (
	"fmt"
	"runtime"
)

func AssetBaseName() (string, error) {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		return "", fmt.Errorf("unsupported GOOS: %s", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported GOARCH: %s", runtime.GOARCH)
	}
	name := fmt.Sprintf("agent_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name, nil
}

func AssetChecksumName() (string, error) {
	base, err := AssetBaseName()
	if err != nil {
		return "", err
	}
	return base + ".sha256", nil
}