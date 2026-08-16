package install

import (
	"fmt"
	"mosona-manager/agent/initsys"
	agentruntime "mosona-manager/agent/runtime"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	serviceName     = "mosona-agent"
	macLaunchdLabel = "cc.mosona.agent"
	macPlistPath    = "/Library/LaunchDaemons/cc.mosona.agent.plist"
)

func installService() error {
	switch runtime.GOOS {
	case "linux":
		return installLinuxService()
	case "darwin":
		return installMacService()
	case "windows":
		return installWindowsService()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// Linux: systemd or OpenRC
func installLinuxService() error {
	switch init, err := initsys.DetectLinux(); {
	case err != nil:
		return err
	case init == initsys.OpenRC:
		return installOpenRCService()
	default:
		return installSystemdService()
	}
}

// Linux systemd
func installSystemdService() error {
	serviceContent := fmt.Sprintf(`[Unit]
Description=Mosona Agent Service
After=network.target

[Service]
Type=simple
ExecStart=%s/mosona-agent run
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target`, agentruntime.InstallDir)

	servicePath := "/etc/systemd/system/mosona-agent.service"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return err
	}

	// Reload systemd and enable/start the service
	commands := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "mosona-agent"},
		{"systemctl", "start", "mosona-agent"},
	}

	for _, args := range commands {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			return fmt.Errorf("failed to execute %v: %w", args, err)
		}
	}

	return nil
}

// Linux OpenRC (Alpine and other OpenRC distributions)
func installOpenRCService() error {
	execPath := filepath.Join(agentruntime.InstallDir, "mosona-agent")
	script := fmt.Sprintf(`#!/sbin/openrc-run

description="Mosona Agent Service"

command=%q
command_args="run"

supervisor="supervise-daemon"
supervise_daemon_args="--respawn-delay 5"
output_log="/var/log/mosona-agent.log"
error_log="/var/log/mosona-agent.log"

depend() {
	after net
}
`, execPath)

	scriptPath := "/etc/init.d/mosona-agent"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return err
	}
	if err := os.Chmod(scriptPath, 0755); err != nil {
		return err
	}

	commands := [][]string{
		{"rc-update", "add", "mosona-agent", "default"},
		{"rc-service", "mosona-agent", "start"},
	}
	for _, args := range commands {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			return fmt.Errorf("failed to execute %v: %w", args, err)
		}
	}

	return nil
}

// macOS Launchd
func installMacService() error {
	execPath := filepath.Join(agentruntime.InstallDir, "mosona-agent")
	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>run</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>/var/log/mosona-agent.log</string>
	<key>StandardErrorPath</key>
	<string>/var/log/mosona-agent.error.log</string>
</dict>
</plist>`, macLaunchdLabel, execPath)

	_ = exec.Command("launchctl", "bootout", "system/"+macLaunchdLabel).Run()
	if err := os.WriteFile(macPlistPath, []byte(plistContent), 0644); err != nil {
		return err
	}

	if err := exec.Command("launchctl", "bootstrap", "system", macPlistPath).Run(); err != nil {
		return fmt.Errorf("failed to bootstrap service: %w", err)
	}
	if err := exec.Command("launchctl", "kickstart", "-k", "system/"+macLaunchdLabel).Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

// Windows
func installWindowsService() error {
	execPath := filepath.Join(agentruntime.InstallDir, "mosona-agent.exe")
	binPath := fmt.Sprintf(`"%s" run`, execPath)

	_ = exec.Command("sc", "stop", serviceName).Run()
	_ = exec.Command("sc", "delete", serviceName).Run()
	_ = exec.Command("sc", "stop", "MosonaAgent").Run()
	_ = exec.Command("sc", "delete", "MosonaAgent").Run()

	cmd := exec.Command("sc", "create", serviceName,
		"binPath=", binPath,
		"start=", "auto",
		"DisplayName=", "Mosona Agent Service")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	// Start
	startCmd := exec.Command("sc", "start", serviceName)
	if err := startCmd.Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

func UninstallService() error {
	switch runtime.GOOS {
	case "linux":
		return uninstallLinuxService()
	case "darwin":
		return uninstallMacService()
	case "windows":
		return uninstallWindowsService()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// Linux: systemd or OpenRC
func uninstallLinuxService() error {
	switch init, err := initsys.DetectLinux(); {
	case err != nil:
		return err
	case init == initsys.OpenRC:
		return uninstallOpenRCService()
	default:
		return uninstallSystemdService()
	}
}

// Linux systemd
func uninstallSystemdService() error {
	commands := [][]string{
		{"systemctl", "stop", "mosona-agent"},
		{"systemctl", "disable", "mosona-agent"},
	}
	for _, args := range commands {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			return fmt.Errorf("failed to execute %v: %w", args, err)
		}
	}

	servicePath := "/etc/systemd/system/mosona-agent.service"
	if err := os.Remove(servicePath); err != nil {
		return err
	}

	// Reload systemd
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	return nil
}

// Linux OpenRC
func uninstallOpenRCService() error {
	_ = exec.Command("rc-service", "mosona-agent", "stop").Run()
	_ = exec.Command("rc-update", "del", "mosona-agent", "default").Run()
	if err := os.Remove("/etc/init.d/mosona-agent"); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// macOS Launchd
func uninstallMacService() error {
	_ = exec.Command("launchctl", "bootout", "system/"+macLaunchdLabel).Run()
	if err := os.Remove(macPlistPath); err != nil {
		return err
	}

	return nil
}

// Windows
func uninstallWindowsService() error {
	currentErr := deleteWindowsService(serviceName)
	legacyErr := deleteWindowsService("MosonaAgent")
	if currentErr != nil && legacyErr != nil {
		return fmt.Errorf("failed to delete service: %w", currentErr)
	}

	return nil
}

func deleteWindowsService(name string) error {
	_ = exec.Command("sc", "stop", name).Run()
	return exec.Command("sc", "delete", name).Run()
}
