package task

import (
	"errors"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
)

func TestCollectSystemUsageRecordsCompleteSample(t *testing.T) {
	var gotCPU, gotMemory int
	err := collectSystemUsage(
		func(time.Duration, bool) ([]float64, error) { return []float64{12.5}, nil },
		func() (*mem.VirtualMemoryStat, error) { return &mem.VirtualMemoryStat{UsedPercent: 25.5}, nil },
		func(cpu, memory int) { gotCPU, gotMemory = cpu, memory },
	)
	if err != nil {
		t.Fatal(err)
	}
	if gotCPU != 1250 || gotMemory != 2550 {
		t.Fatalf("recorded usage = (%d, %d), want (1250, 2550)", gotCPU, gotMemory)
	}
}

func TestCollectSystemUsageRejectsIncompleteSamples(t *testing.T) {
	wantErr := errors.New("collection failed")
	tests := map[string]struct {
		cpu func(time.Duration, bool) ([]float64, error)
		mem func() (*mem.VirtualMemoryStat, error)
	}{
		"CPU error": {
			cpu: func(time.Duration, bool) ([]float64, error) { return nil, wantErr },
			mem: func() (*mem.VirtualMemoryStat, error) { return &mem.VirtualMemoryStat{}, nil },
		},
		"empty CPU": {
			cpu: func(time.Duration, bool) ([]float64, error) { return nil, nil },
			mem: func() (*mem.VirtualMemoryStat, error) { return &mem.VirtualMemoryStat{}, nil },
		},
		"memory error": {
			cpu: func(time.Duration, bool) ([]float64, error) { return []float64{1}, nil },
			mem: func() (*mem.VirtualMemoryStat, error) { return nil, wantErr },
		},
		"nil memory": {
			cpu: func(time.Duration, bool) ([]float64, error) { return []float64{1}, nil },
			mem: func() (*mem.VirtualMemoryStat, error) { return nil, nil },
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			recorded := false
			if err := collectSystemUsage(test.cpu, test.mem, func(int, int) { recorded = true }); err == nil {
				t.Fatal("incomplete sample was accepted")
			}
			if recorded {
				t.Fatal("incomplete sample was recorded")
			}
		})
	}
}
