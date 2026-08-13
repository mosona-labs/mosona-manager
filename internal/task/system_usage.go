package task

import (
	"fmt"
	"log"
	"mosona-manager/internal/influx"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

func SystemUsage() {
	if err := collectSystemUsage(cpu.Percent, mem.VirtualMemory, influx.UsageRecordAdd); err != nil {
		log.Println("System usage error:", err)
	}
}

func collectSystemUsage(
	cpuPercent func(time.Duration, bool) ([]float64, error),
	virtualMemory func() (*mem.VirtualMemoryStat, error),
	record func(int, int),
) error {
	percents, err := cpuPercent(500*time.Millisecond, false)
	if err != nil {
		return fmt.Errorf("CPU percent: %w", err)
	}
	if len(percents) == 0 {
		return fmt.Errorf("CPU percent: empty result")
	}
	vm, err := virtualMemory()
	if err != nil {
		return fmt.Errorf("memory percent: %w", err)
	}
	if vm == nil {
		return fmt.Errorf("memory percent: empty result")
	}

	record(
		int(percents[0]*100),
		int(vm.UsedPercent*100),
	)
	return nil
}
