//go:build darwin

package telemetry

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"mosona-manager/agent/types"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func CollectHostInfo(ctx context.Context) types.Info {
	var info types.Info

	info.SystemVersion = firstNonEmpty(
		strings.TrimSpace(runCmd(ctx, "sw_vers", "-productName")),
		"macOS",
	)

	uptimeSec, _ := darwinUptimeSeconds()
	if uptimeSec < 0 {
		uptimeSec = 0
	}
	info.Uptime = uptimeSec

	info.CpuName = strings.TrimSpace(firstNonEmpty(
		sysctlString("machdep.cpu.brand_string"),
		sysctlString("hw.model"),
	))

	// Physical & Logical CPU counts
	info.CpuC = int(sysctlInt64("hw.physicalcpu", 0))
	info.CpuT = int(sysctlInt64("hw.logicalcpu", 0))
	if info.CpuC <= 0 {
		info.CpuC = 1
	}
	if info.CpuT <= 0 {
		info.CpuT = info.CpuC
	}

	info.KernelVersion = strings.TrimSpace(firstNonEmpty(
		runCmd(ctx, "uname", "-r"),
	))

	info.Architecture = runtime.GOARCH

	return info
}

func darwinUptimeSeconds() (int64, error) {
	raw, err := sysctlRaw("kern.boottime")
	if err != nil || len(raw) < 8 {
		return 0, errors.New("cannot read kern.boottime")
	}

	sec := int64(binary.LittleEndian.Uint64(raw[:8]))
	if sec <= 0 {
		return 0, errors.New("invalid boottime")
	}

	now := time.Now().Unix()
	return now - sec, nil
}

func sysctlString(name string) string {
	out := strings.TrimSpace(runCmd(context.Background(), "sysctl", "-n", name))
	return out
}

func sysctlInt64(name string, def int64) int64 {
	out := strings.TrimSpace(runCmd(context.Background(), "sysctl", "-n", name))
	if out == "" {
		return def
	}
	v, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return def
	}
	return v
}

func sysctlRaw(name string) ([]byte, error) {
	cmd := exec.Command("sysctl", "-b", name)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() { _ = cmd.Wait() }()
	return io.ReadAll(stdout)
}
