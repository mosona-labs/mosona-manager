package alerttasks

import (
	"fmt"
	"mosona-manager/internal/_type"
	"time"
)

type metricCalculator func(statuses []*_type.ServerStatusType, startTime time.Time) (float64, int)

func shouldNotify(currentStatus bool, r *alertRule) bool {
	if !currentStatus {
		return false
	}

	if r.lastStatus == nil {
		return true
	}
	if *r.lastStatus != currentStatus {
		return true
	}
	if r.lastNotifyAt == nil || time.Since(*r.lastNotifyAt) > time.Hour {
		return true
	}

	return false
}

func (a *alertInstance) checkMetricAlert(
	serverId int64,
	r *alertRule,
	calculator metricCalculator,
	alertTitle string,
	messageFormatter func(value float64, duration int, threshold int) string,
) (bool, *time.Time) {
	value, count := calculator(a.statuses, r.startTime)

	if count == 0 {
		return false, r.lastNotifyAt
	}

	avgValue := value / float64(count)
	currentStatus := avgValue >= float64(r.threshold)

	now := time.Now()
	if shouldNotify(currentStatus, r) {
		a.notifyAll(serverId, alertTitle, messageFormatter(avgValue, r.forDuration, r.threshold))
		return currentStatus, &now
	}

	return currentStatus, r.lastNotifyAt
}

func (a *alertInstance) checkStatusAlert(serverId int64, r *alertRule) (bool, *time.Time) {
	// Check if server has any status data after startTime
	currentStatus := false
	for _, item := range a.statuses {
		if item.Time.After(r.startTime) {
			currentStatus = true
			break
		}
	}

	now := time.Now()

	// Determine if notification should be sent
	notify := false
	if r.lastStatus == nil {
		// First time checking - notify if down
		notify = !currentStatus
	} else {
		// Status changed or recurring notification needed
		statusChanged := *r.lastStatus != currentStatus
		recurringNotification := currentStatus && r.lastNotifyAt != nil && time.Since(*r.lastNotifyAt) > time.Hour
		notify = statusChanged || recurringNotification
	}

	if notify {
		if currentStatus {
			a.notifyAll(serverId, "Server Up", "The server is now UP.")
		} else {
			a.notifyAll(serverId, "Server Down", "No response for 10-minutes.")
		}
		return currentStatus, &now
	}

	return currentStatus, r.lastNotifyAt
}

func (a *alertInstance) checkCPUUsageAlert(serverId int64, r *alertRule) (bool, *time.Time) {
	calculator := func(statuses []*_type.ServerStatusType, startTime time.Time) (float64, int) {
		var sum float64
		var count int
		for _, item := range statuses {
			if item.Time.After(startTime) {
				sum += item.CPU
				count++
			}
		}
		return sum, count
	}

	return a.checkMetricAlert(
		serverId,
		r,
		calculator,
		"CPU usage exceeded threshold",
		func(value float64, duration int, threshold int) string {
			return fmt.Sprintf("The %d-minutes average CPU usage reached %.2f%%, exceeding the configured threshold of %d%%",
				duration, value, threshold)
		},
	)
}

func (a *alertInstance) checkMemoryUsageAlert(serverId int64, r *alertRule) (bool, *time.Time) {
	calculator := func(statuses []*_type.ServerStatusType, startTime time.Time) (float64, int) {
		var sum float64
		var count int
		for _, item := range statuses {
			if item.Time.After(startTime) {
				sum += (item.MemUsedMB / item.MemTotalMB) * 100 // Convert to percentage
				count++
			}
		}
		return sum, count
	}

	return a.checkMetricAlert(
		serverId,
		r,
		calculator,
		"Memory usage exceeded threshold",
		func(value float64, duration int, threshold int) string {
			return fmt.Sprintf("The %d-minutes average Memory usage reached %.2f%%, exceeding the configured threshold of %d%%",
				duration, value, threshold)
		},
	)
}

func (a *alertInstance) checkDiskUsageAlert(serverId int64, r *alertRule) (bool, *time.Time) {
	type diskStat struct {
		sum   float64
		count int
	}
	perDisk := make(map[string]*diskStat)
	for _, item := range a.statuses {
		if !item.Time.After(r.startTime) {
			continue
		}
		for _, d := range item.Disks {
			if d.TotalGB <= 0 {
				continue
			}
			key := d.MountPoint
			if key == "" {
				key = "/"
			}
			stat, ok := perDisk[key]
			if !ok {
				stat = &diskStat{}
				perDisk[key] = stat
			}
			stat.sum += (d.UsedGB / d.TotalGB) * 100
			stat.count++
		}
	}

	var currentStatus bool
	var mountPoint string
	var avgValue float64
	for mp, stat := range perDisk {
		if stat.count == 0 {
			continue
		}
		avg := stat.sum / float64(stat.count)
		if avg > avgValue {
			avgValue = avg
			mountPoint = mp
		}
		if avg >= float64(r.threshold) {
			currentStatus = true
		}
	}

	if len(perDisk) == 0 {
		return false, r.lastNotifyAt
	}

	now := time.Now()
	if shouldNotify(currentStatus, r) {
		a.notifyAll(
			serverId,
			"Disk usage exceeded threshold",
			fmt.Sprintf(
				"The %d-minutes average disk usage for %s reached %.2f%%, exceeding the configured threshold of %d%%",
				r.forDuration,
				mountPoint,
				avgValue,
				r.threshold,
			),
		)
		return currentStatus, &now
	}

	return currentStatus, r.lastNotifyAt
}

func (a *alertInstance) checkExpiryReminderAlert(serverId int64, r *alertRule) (bool, *time.Time) {
	serverExpiry, ok := a.expiryMap[serverId]
	if !ok || serverExpiry.EndTime == nil || serverExpiry.AutoRenew {
		return false, r.lastNotifyAt
	}

	remaining := time.Until(*serverExpiry.EndTime)
	currentStatus := remaining > 0 && remaining <= time.Duration(r.threshold)*24*time.Hour
	if !currentStatus {
		return false, r.lastNotifyAt
	}

	if r.lastStatus != nil && *r.lastStatus {
		return true, r.lastNotifyAt
	}

	daysLeft := int((remaining + 24*time.Hour - 1) / (24 * time.Hour))
	if daysLeft < 1 {
		daysLeft = 1
	}
	now := time.Now()
	a.notifyAll(
		serverId,
		"Server expiry reminder",
		fmt.Sprintf(
			"This server is not set to auto-renew and will expire in %d day(s) on %s.",
			daysLeft,
			serverExpiry.EndTime.Format("2006-01-02 15:04 MST"),
		),
	)
	return true, &now
}

func (a *alertInstance) checkReadIOPSAlert(serverId int64, r *alertRule) (bool, *time.Time) {
	calculator := func(statuses []*_type.ServerStatusType, startTime time.Time) (float64, int) {
		var sum float64
		var count int
		for _, item := range statuses {
			if item.Time.After(startTime) {
				sum += item.DiskReadIOPS
				count++
			}
		}
		return sum, count
	}

	return a.checkMetricAlert(
		serverId,
		r,
		calculator,
		"Read IOPS exceeded threshold",
		func(value float64, duration int, threshold int) string {
			return fmt.Sprintf("The %d-minutes average Read IOPS reached %.1f, exceeding the configured threshold of %d IOPS",
				duration, value, threshold)
		},
	)
}

func (a *alertInstance) checkWriteIOPSAlert(serverId int64, r *alertRule) (bool, *time.Time) {
	calculator := func(statuses []*_type.ServerStatusType, startTime time.Time) (float64, int) {
		var sum float64
		var count int
		for _, item := range statuses {
			if item.Time.After(startTime) {
				sum += item.DiskWriteIOPS
				count++
			}
		}
		return sum, count
	}

	return a.checkMetricAlert(
		serverId,
		r,
		calculator,
		"Write IOPS exceeded threshold",
		func(value float64, duration int, threshold int) string {
			return fmt.Sprintf("The %d-minutes average Write IOPS reached %.1f, exceeding the configured threshold of %d IOPS",
				duration, value, threshold)
		},
	)
}

func (a *alertInstance) checkBandwidthAlert(serverId int64, r *alertRule) (bool, *time.Time) {
	calculator := func(statuses []*_type.ServerStatusType, startTime time.Time) (float64, int) {
		var sum float64 // Mbps
		var count int
		for _, item := range statuses {
			if item.Time.After(startTime) {
				sum += (item.RxKibS + item.TxKibS) * 8 / 1024
				count++
			}
		}
		return sum, count
	}

	return a.checkMetricAlert(
		serverId,
		r,
		calculator,
		"Bandwidth usage exceeded threshold",
		func(value float64, duration int, threshold int) string {
			return fmt.Sprintf("The %d-minutes average Bandwidth usage reached %.2f Mbps, exceeding the configured threshold of %d Mbps",
				duration, value, threshold)
		},
	)
}
