package task

import (
	"log"
	"mosona-manager/internal/influx"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

func SystemUsage() {
	percents, err := cpu.Percent(500*time.Millisecond, false)
	if err != nil {
		log.Println("System usage error: cpu percent error:", err)
	}
	vm, err := mem.VirtualMemory()
	if err != nil {
		log.Println("System usage error: memory percent error:", err)
	}

	influx.UsageRecordAdd(
		int(percents[0]*100),
		int(vm.UsedPercent*100),
	)
}
