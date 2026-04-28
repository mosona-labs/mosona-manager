package install

import (
	"fmt"
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

// Linux systemd
func installLinuxService() error {
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

	_ = exec.Command("launchctl", "unload", macPlistPath).Run()
	if err := os.WriteFile(macPlistPath, []byte(plistContent), 0644); err != nil {
		return err
	}

	// Load and start the service
	cmd := exec.Command("launchctl", "load", macPlistPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load service: %w", err)
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

// Linux systemd
func uninstallLinuxService() error {
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

// macOS Launchd
func uninstallMacService() error {
	cmd := exec.Command("launchctl", "unload", macPlistPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to unload service: %w", err)
	}
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
