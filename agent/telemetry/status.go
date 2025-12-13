package telemetry

import (
	"mosona-manager/agent/types"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

type Monitor struct {
	mu sync.Mutex

	diskPath    string
	diskTotalGB float64
	memTotalMB  float64
	swapTotalMB float64

	lastDiskIO    map[string]disk.IOCountersStat
	lastNetIO     net.IOCountersStat
	lastCheckTime time.Time
}

func NewMonitor() *Monitor {
	m := &Monitor{
		lastDiskIO:    make(map[string]disk.IOCountersStat),
		lastCheckTime: time.Now(),
	}

	m.diskPath = "/"
	if runtime.GOOS == "windows" {
		m.diskPath = "C:"
	}

	if diskStat, err := disk.Usage(m.diskPath); err == nil {
		m.diskTotalGB = float64(diskStat.Total) / 1024 / 1024 / 1024
	}

	if vmStat, err := mem.VirtualMemory(); err == nil {
		m.memTotalMB = float64(vmStat.Total) / 1024 / 1024
	}

	if swapStat, err := mem.SwapMemory(); err == nil {
		m.swapTotalMB = float64(swapStat.Total) / 1024 / 1024
	}

	return m
}

func (m *Monitor) Snapshot() (*types.Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := &types.Status{}
	now := time.Now()
	elapsed := now.Sub(m.lastCheckTime).Seconds()
	if elapsed == 0 {
		elapsed = 1
	}

	if cpuPercent, err := cpu.Percent(1000*time.Millisecond, false); err == nil && len(cpuPercent) > 0 {
		s.CPU = cpuPercent[0]
	}
	s.MemTotalMB = m.memTotalMB
	if vmStat, err := mem.VirtualMemory(); err == nil {
		s.MemUsedMB = float64(vmStat.Used) / 1024 / 1024
		if m.memTotalMB == 0 {
			m.memTotalMB = float64(vmStat.Total) / 1024 / 1024
			s.MemTotalMB = m.memTotalMB
		}
	}
	s.SwapTotalMB = m.swapTotalMB
	if swapStat, err := mem.SwapMemory(); err == nil {
		s.SwapUsedMB = float64(swapStat.Used) / 1024 / 1024
		if m.swapTotalMB == 0 {
			m.swapTotalMB = float64(swapStat.Total) / 1024 / 1024
			s.SwapTotalMB = m.swapTotalMB
		}
	}
	s.DiskTotalGB = m.diskTotalGB
	if diskStat, err := disk.Usage(m.diskPath); err == nil {
		s.DiskUsedGB = float64(diskStat.Used) / 1024 / 1024 / 1024
	}
	if ioCounters, err := disk.IOCounters(); err == nil {
		for name, io := range ioCounters {
			if strings.HasPrefix(name, "ram") ||
				strings.HasPrefix(name, "loop") ||
				strings.HasPrefix(name, "fd") {
				continue
			}

			if last, ok := m.lastDiskIO[name]; ok {
				readBytes := float64(io.ReadBytes - last.ReadBytes)
				writeBytes := float64(io.WriteBytes - last.WriteBytes)
				readCount := float64(io.ReadCount - last.ReadCount)
				writeCount := float64(io.WriteCount - last.WriteCount)

				s.DiskReadKibS += (readBytes / 1024) / elapsed
				s.DiskWriteKibS += (writeBytes / 1024) / elapsed
				s.DiskReadIOPS += readCount / elapsed
				s.DiskWriteIOPS += writeCount / elapsed
			}
			m.lastDiskIO[name] = io
		}
	}
	if netIO, err := net.IOCounters(true); err == nil {
		var totalRecv, totalSent uint64
		for _, io := range netIO {
			name := io.Name
			if name == "lo" ||
				strings.HasPrefix(name, "veth") ||
				strings.HasPrefix(name, "docker") ||
				strings.HasPrefix(name, "br-") {
				continue
			}
			totalRecv += io.BytesRecv
			totalSent += io.BytesSent
		}

		s.RxTotalMB = float64(totalRecv) / 1024 / 1024
		s.TxTotalMB = float64(totalSent) / 1024 / 1024

		if m.lastNetIO.BytesRecv > 0 {
			rxBytes := float64(totalRecv - m.lastNetIO.BytesRecv)
			txBytes := float64(totalSent - m.lastNetIO.BytesSent)
			s.RxKibS = (rxBytes / 1024) / elapsed
			s.TxKibS = (txBytes / 1024) / elapsed
		}
		m.lastNetIO = net.IOCountersStat{
			BytesRecv: totalRecv,
			BytesSent: totalSent,
		}
	}
	if connections, err := net.Connections("all"); err == nil {
		for _, conn := range connections {
			if conn.Type == 1 {
				s.TCPTotal++
			} else if conn.Type == 2 {
				s.UDPTotal++
			}
		}
	}

	m.lastCheckTime = now
	return s, nil
}
