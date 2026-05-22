package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"mosona-manager/agent/active"
	"mosona-manager/agent/config"
	"mosona-manager/agent/install"
	"mosona-manager/agent/passive"
	"mosona-manager/agent/runhost"
	"mosona-manager/agent/runtime"
	"mosona-manager/agent/service"
	"os"
	"strconv"
	"strings"
)

const Logo = `┳┳┓           ┳┳┓            
┃┃┃┏┓┏┏┓┏┓┏┓  ┃┃┃┏┓┏┓┏┓┏┓┏┓┏┓
┛ ┗┗┛┛┗┛┛┗┗┻  ┛ ┗┗┻┛┗┗┻┗┫┗ ┛ 
                        ┛`

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]

	switch command {
	case "run":
		handleRun()
	case "start":
		handleStart()
	case "stop":
		handleStop()
	case "restart":
		handleRestart()
	case "status":
		handleStatus()
	case "install":
		handleInstall()
	case "uninstall":
		handleUninstall()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: mosona-agent <command> [args]")
	fmt.Println("Commands:")
	fmt.Println("  run                    - Start the agent in foreground (install required)")
	fmt.Println("  start                  - Service start (install required)")
	fmt.Println("  stop                   - Service stop (install required)")
	fmt.Println("  restart                - Service restart (install required)")
	fmt.Println("  status                 - Show service status (install required)")
	fmt.Println("  uninstall              - Uninstall the agent service")
	fmt.Println("  install <mode> <args>  - Install and configure the agent")
}

func handleRun() {
	if err := config.Load(); err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(Logo)
	fmt.Println("⇨ Mosona manager agent v" + runtime.Version + " starting...")

	if err := runhost.Run("mosona-agent", func() {
		switch config.Current.Mode {
		case "passive":
			fmt.Println("⇨ Running in passive mode, connecting to hub:", config.Current.Hub)
			passive.Run()
		case "active":
			fmt.Printf("⇨ Running in active mode, listening on %s:%d\n", config.Current.Host, config.Current.Port)
			active.Run()
		default:
			fmt.Println("Unknown mode:", config.Current.Mode)
			os.Exit(1)
		}
	}); err != nil {
		fmt.Println("Failed to start run host:", err)
		os.Exit(1)
	}
}

func handleStart() {
	if output, err := service.Start(); err != nil {
		fmt.Printf("Failed to start service: %v\n%s\n", err, string(output))
		os.Exit(1)
	}
}

func handleStop() {
	if output, err := service.Stop(); err != nil {
		fmt.Printf("Failed to stop service: %v\n%s\n", err, string(output))
		os.Exit(1)
	}
}

func handleRestart() {
	if output, err := service.Restart(); err != nil {
		fmt.Printf("Failed to restart service: %v\n%s\n", err, string(output))
		os.Exit(1)
	}
}

func handleStatus() {
	if output, err := service.Status(); err != nil {
		fmt.Printf("Failed to get service status: %v\n%s\n", err, string(output))
		os.Exit(1)
	} else {
		fmt.Printf("%s", string(output))
	}
}

func handleInstall() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: agent install <active|passive> [--no-monitor] [--no-terminal] <args>")
		os.Exit(1)
	}

	mode := os.Args[2]
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	noMonitor := fs.Bool("no-monitor", false, "Disable monitoring")
	noTerminal := fs.Bool("no-terminal", false, "Disallow terminal")
	ipv4 := fs.Bool("ipv4", false, "Prefer IPv4 for outbound hub connections")
	ipv6 := fs.Bool("ipv6", false, "Prefer IPv6 for outbound hub connections")
	if fs.Parse(os.Args[3:]) != nil {
		fmt.Println("Failed to parse flags")
		return
	}
	if *ipv4 && *ipv6 {
		fmt.Println("--ipv4 and --ipv6 cannot be used together")
		os.Exit(1)
	}

	switch mode {
	case "active":
		args := fs.Args()
		if len(args) < 4 {
			fmt.Println("Usage: agent install active [--no-monitor] [--no-terminal] <uid> <public_key> <host> <port>")
			os.Exit(1)
		}

		uid := args[0]
		publicKey := args[1]
		host := args[2]
		port, _ := strconv.Atoi(args[3])

		// Validation
		if uid == "" || publicKey == "" || host == "" || port == 0 {
			fmt.Println("Invalid args")
			os.Exit(1)
		}
		pubKey, err := base64.StdEncoding.DecodeString(publicKey)
		if err != nil || len(pubKey) == 0 {
			fmt.Println("Invalid public key")
			os.Exit(1)
		}

		if err := install.Active(uid, string(pubKey), host, port, *noMonitor, *noTerminal); err != nil {
			fmt.Printf("Installation failed: %v\n", err)
			os.Exit(1)
		}
	case "passive":
		args := fs.Args()
		if len(args) < 2 {
			fmt.Println("Usage: agent install passive [--no-monitor] [--no-terminal] [--ipv4|--ipv6] <hub> <enroll_key>")
			os.Exit(1)
		}

		hub := args[0]
		token := args[1]
		if hub[len(hub)-1] == '/' {
			hub = hub[:len(hub)-1]
		}

		// Validation
		if !strings.HasPrefix(hub, "http://") && !strings.HasPrefix(hub, "https://") {
			fmt.Println("Invalid hub URL")
			os.Exit(1)
		}
		if token == "" {
			fmt.Println("Invalid enroll key")
			os.Exit(1)
		}

		ipPreference := ""
		if *ipv4 {
			ipPreference = "ipv4"
		} else if *ipv6 {
			ipPreference = "ipv6"
		}

		if err := install.Passive(hub, token, *noMonitor, *noTerminal, ipPreference); err != nil {
			fmt.Printf("Installation failed: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Unknown install mode: %s\n", mode)
		os.Exit(1)
	}
}

func handleUninstall() {
	if err := install.UninstallService(); err != nil {
		fmt.Printf("Uninstallation failed: %v\n", err)
		os.Exit(1)
	}
	if err := os.RemoveAll(runtime.InstallDir); err != nil {
		fmt.Printf("Failed to remove agent files: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Agent service uninstalled successfully.")
}
