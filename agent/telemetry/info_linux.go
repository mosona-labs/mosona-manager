//go:build linux

package telemetry

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"

	"mosona-manager/agent/types"
)

func CollectHostInfo(ctx context.Context) types.Info {
	var info types.Info

	info.SystemVersion = linuxSystemVersionFirstWord()

	info.Uptime = linuxUptimeSeconds()

	cpuName, cpuC, cpuT := linuxCPUInfo()
	info.CpuName = cpuName
	info.CpuC = cpuC
	info.CpuT = cpuT

	info.KernelVersion = strings.TrimSpace(firstNonEmpty(
		runCmd(ctx, "uname", "-r"),
	))

	info.Architecture = strings.TrimSpace(firstNonEmpty(
		runCmd(ctx, "uname", "-m"),
	))

	return info
}

func linuxSystemVersionFirstWord() string {
	// Prefer /etc/os-release NAME
	name := ""
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		sc := bufio.NewScanner(strings.NewReader(string(b)))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "NAME=") {
				v := strings.TrimPrefix(line, "NAME=")
				v = strings.Trim(v, `"'`)
				name = v
				break
			}
		}
	}
	// Fallback /etc/redhat-release
	if name == "" {
		if b, err := os.ReadFile("/etc/redhat-release"); err == nil {
			name = strings.TrimSpace(string(b))
		}
	}
	if name == "" {
		name = "Linux"
	}

	// Match your awk '{print $1}'
	fields := strings.Fields(name)
	if len(fields) > 0 {
		return fields[0]
	}
	return name
}

func linuxUptimeSeconds() int64 {
	// /proc/uptime => "12345.67 89012.34"
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0
	}
	// take integer part before dot
	s := fields[0]
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return 0
	}
	num, _ := strconv.ParseInt(s, 10, 64)
	return num
}

func linuxCPUInfo() (cpuName string, cpuC int, cpuT int) {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "", 1, 1
	}
	defer f.Close()

	sc := bufio.NewScanner(f)

	// For x86: model name
	// For ARM: Hardware / Model / cpu model
	var modelName, hardware, model, cpuModel string

	threads := 0
	seenCorePairs := map[string]struct{}{} // physical:core
	seenPhysical := map[string]struct{}{}  // physical id values

	var currentPhysical string

	for sc.Scan() {
		line := sc.Text()

		// CPU name fields - collect all possible values
		if modelName == "" && strings.HasPrefix(line, "model name") {
			if v := afterColon(line); v != "" {
				modelName = v
			}
		}
		if hardware == "" && strings.HasPrefix(line, "Hardware") {
			if v := afterColon(line); v != "" {
				hardware = v
			}
		}
		if model == "" && strings.HasPrefix(line, "Model") {
			if v := afterColon(line); v != "" {
				model = v
			}
		}
		if cpuModel == "" && strings.HasPrefix(line, "cpu model") {
			if v := afterColon(line); v != "" {
				cpuModel = v
			}
		}

		// Logical threads count
		if strings.HasPrefix(line, "processor") {
			threads++
		}

		// Physical/core counting (x86 usually)
		if strings.HasPrefix(line, "physical id") {
			currentPhysical = strings.TrimSpace(afterColon(line))
			if currentPhysical != "" {
				seenPhysical[currentPhysical] = struct{}{}
			}
		}
		if strings.HasPrefix(line, "core id") {
			coreID := strings.TrimSpace(afterColon(line))
			if currentPhysical != "" && coreID != "" {
				seenCorePairs[currentPhysical+":"+coreID] = struct{}{}
			}
		}
	}

	if threads <= 0 {
		threads = 1
	}

	// CPU name priority: model name > Hardware > Model > cpu model
	name := modelName
	if name == "" {
		name = hardware
	}
	if name == "" {
		name = model
	}
	if name == "" {
		name = cpuModel
	}
	if name == "" {
		name = "Unknown CPU"
	}
	name = strings.TrimSpace(name)

	cores := 0
	if len(seenCorePairs) > 0 {
		// Total unique (physical id, core id) pairs
		total := len(seenCorePairs)

		// Sockets count
		sockets := len(seenPhysical)
		if sockets == 0 {
			sockets = 1
		}

		// Cores per socket
		coresPerSocket := total / sockets
		if coresPerSocket > 0 {
			cores = coresPerSocket
		}
	}

	return name, cores, threads
}

func afterColon(line string) string {
	if i := strings.IndexByte(line, ':'); i >= 0 && i+1 < len(line) {
		return strings.TrimSpace(line[i+1:])
	}
	return ""
}
