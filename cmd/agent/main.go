package main

import (
	"flag"
	"fmt"
	"mosona-manager/agent/config"
	"mosona-manager/agent/install"
	"mosona-manager/agent/passive"
	"mosona-manager/agent/runtime"
	"mosona-manager/agent/service"
	"os"
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

	switch config.Current.Mode {
	case "passive":
		fmt.Println("⇨ Running in passive mode, connecting to hub:", config.Current.Hub)
		passive.Run()
	default:
		fmt.Println("Unknown mode:", config.Current.Mode)
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
		fmt.Println("Usage: agent install <active|passive> [--no-monitor] <args>")
		os.Exit(1)
	}

	mode := os.Args[2]
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	noMonitor := fs.Bool("no-monitor", false, "Disable monitoring")
	if fs.Parse(os.Args[3:]) != nil {
		fmt.Println("Failed to parse flags")
		return
	}
	noTerminal := fs.Bool("no-terminal", false, "Disallow terminal")
	if fs.Parse(os.Args[3:]) != nil {
		fmt.Println("Failed to parse flags")
		return
	}

	switch mode {
	case "active":
		// TODO
	case "passive":
		args := fs.Args()
		if len(args) < 2 {
			fmt.Println("Usage: agent install passive [--no-monitor] [--no-terminal] <hub> <enroll_key>")
			os.Exit(1)
		}

		hub := args[0]
		token := args[1]
		if hub[len(hub)-1] == '/' {
			hub = hub[:len(hub)-1]
		}

		if err := install.Passive(hub, token, *noMonitor, *noTerminal); err != nil {
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
