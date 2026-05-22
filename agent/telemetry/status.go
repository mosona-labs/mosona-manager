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
		"tmpfs":       true,
		"devtmpfs":    true,
		"devfs":       true,
		"squashfs":    true,
		"overlay":     true,
		"iso9660":     true,
		"udf":         true,
		"efivarfs":    true,
		"binfmt_misc": true,
		"autofs":      true,
		"fuse.portal": true,
		"nsfs":        true,
		"proc":        true,
		"sysfs":       true,
		"cgroup":      true,
		"cgroup2":     true,
		"pstore":      true,
		"debugfs":     true,
		"tracefs":     true,
		"securityfs":  true,
		"configfs":    true,
		"fusectl":     true,
		"hugetlbfs":   true,
		"mqueue":      true,
		"bpf":         true,
		"ramfs":       true,
	}

	excludeMountPrefixes := []string{"/boot", "/snap/", "/sys", "/proc", "/dev", "/run"}

	partitions, err := disk.Partitions(false)
	if err != nil {
		// Fallback to root partition only
		if usage, e := disk.Usage(fallbackUsagePath()); e == nil {
			return []types.DiskInfo{{
				MountPoint: fallbackUsagePath(),
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

		if runtime.GOOS == "darwin" && isDarwinSystemVolume(p.Mountpoint) {
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
		if usage, e := disk.Usage(fallbackUsagePath()); e == nil {
			result = append(result, types.DiskInfo{
				MountPoint: fallbackUsagePath(),
				TotalGB:    float64(usage.Total) / 1024 / 1024 / 1024,
				UsedGB:     float64(usage.Used) / 1024 / 1024 / 1024,
			})
		}
	}

	return result
}

func fallbackUsagePath() string {
	if runtime.GOOS == "windows" {
		return `C:\`
	}
	return "/"
}

func isDarwinSystemVolume(mountpoint string) bool {
	if strings.HasPrefix(mountpoint, "/System/Volumes/") {
		return true
	}

	switch mountpoint {
	case "/Volumes/Recovery",
		"/Volumes/Preboot",
		"/Volumes/Update",
		"/Volumes/VM",
		"/Volumes/Data":
		return true
	default:
		return false
	}
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
				readBytes := deltaUint64(io.ReadBytes, last.ReadBytes)
				writeBytes := deltaUint64(io.WriteBytes, last.WriteBytes)
				readCount := deltaUint64(io.ReadCount, last.ReadCount)
				writeCount := deltaUint64(io.WriteCount, last.WriteCount)

				s.DiskReadKibS += (float64(readBytes) / 1024) / elapsed
				s.DiskWriteKibS += (float64(writeBytes) / 1024) / elapsed
				s.DiskReadIOPS += float64(readCount) / elapsed
				s.DiskWriteIOPS += float64(writeCount) / elapsed
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
			rxBytes := deltaUint64(totalRecv, m.lastNetIO.BytesRecv)
			txBytes := deltaUint64(totalSent, m.lastNetIO.BytesSent)
			s.RxKibS = (float64(rxBytes) / 1024) / elapsed
			s.TxKibS = (float64(txBytes) / 1024) / elapsed
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

func deltaUint64(current, previous uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}
