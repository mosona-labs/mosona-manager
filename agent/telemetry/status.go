package telemetry

import (
	"mosona-manager/agent/types"
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

	if vmStat, err := mem.VirtualMemory(); err == nil {
		m.memTotalMB = float64(vmStat.Total) / 1024 / 1024
	}

	if swapStat, err := mem.SwapMemory(); err == nil {
		m.swapTotalMB = float64(swapStat.Total) / 1024 / 1024
	}

	return m
}

// qualifiedPartitions returns disk partitions excluding boot/efi and
// non-root partitions smaller than 5GB.
func qualifiedPartitions() []types.DiskInfo {
	excludeFsTypes := map[string]bool{
		"tmpfs":     true,
		"devtmpfs":  true,
		"devfs":     true,
		"squashfs":  true,
		"overlay":   true,
		"iso9660":   true,
		"udf":       true,
		"efivarfs":  true,
		"binfmt_misc": true,
		"autofs":    true,
		"fuse.portal": true,
		"nsfs":      true,
		"proc":      true,
		"sysfs":     true,
		"cgroup":    true,
		"cgroup2":   true,
		"pstore":    true,
		"debugfs":   true,
		"tracefs":   true,
		"securityfs": true,
		"configfs":  true,
		"fusectl":   true,
		"hugetlbfs": true,
		"mqueue":    true,
		"bpf":       true,
		"ramfs":     true,
	}

	excludeMountPrefixes := []string{"/boot", "/snap/", "/sys", "/proc", "/dev", "/run"}

	partitions, err := disk.Partitions(false)
	if err != nil {
		// Fallback to root partition only
		if usage, e := disk.Usage("/"); e == nil {
			return []types.DiskInfo{{
				MountPoint: "/",
				TotalGB:    float64(usage.Total) / 1024 / 1024 / 1024,
				UsedGB:     float64(usage.Used) / 1024 / 1024 / 1024,
			}}
		}
		return nil
	}

	const minSizeGB = 5.0
	var result []types.DiskInfo
	seen := make(map[string]bool)

	for _, p := range partitions {
		if excludeFsTypes[p.Fstype] {
			continue
		}

		skip := false
		for _, prefix := range excludeMountPrefixes {
			if strings.HasPrefix(p.Mountpoint, prefix) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		if seen[p.Mountpoint] {
			continue
		}
		seen[p.Mountpoint] = true

		usage, err := disk.Usage(p.Mountpoint)
		if err != nil || usage.Total == 0 {
			continue
		}

		totalGB := float64(usage.Total) / 1024 / 1024 / 1024

		// Keep root partition regardless of size; skip others < 5GB
		if p.Mountpoint != "/" && totalGB < minSizeGB {
			continue
		}

		result = append(result, types.DiskInfo{
			MountPoint: p.Mountpoint,
			TotalGB:    totalGB,
			UsedGB:     float64(usage.Used) / 1024 / 1024 / 1024,
		})
	}

	if len(result) == 0 {
		if usage, e := disk.Usage("/"); e == nil {
			result = append(result, types.DiskInfo{
				MountPoint: "/",
				TotalGB:    float64(usage.Total) / 1024 / 1024 / 1024,
				UsedGB:     float64(usage.Used) / 1024 / 1024 / 1024,
			})
		}
	}

	return result
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
	s.Disks = qualifiedPartitions()
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
