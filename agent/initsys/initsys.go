package initsys

import (
	"errors"
	"os"
	"os/exec"
)

const (
	Systemd = "systemd"
	OpenRC  = "openrc"
)

// DetectLinux returns the init system that manages services on this Linux host.
func DetectLinux() (string, error) {
	if fi, err := os.Stat("/run/systemd/system"); err == nil && fi.IsDir() {
		return Systemd, nil
	}
	if _, err := exec.LookPath("rc-service"); err == nil {
		return OpenRC, nil
	}
	if _, err := os.Stat("/sbin/rc-service"); err == nil {
		return OpenRC, nil
	}
	return "", errors.New("unsupported init system: neither systemd nor OpenRC detected")
}
