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
				sum += item.CPU * 100 // Convert to percentage
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
			return fmt.Sprintf("%.2f%% CPU usage over the last %d minutes exceeded the threshold of %d%%.",
				value, duration, threshold)
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
			return fmt.Sprintf("%.2f%% Memory usage over the last %d minutes exceeded the threshold of %d%%.",
				value, duration, threshold)
		},
	)
}

func (a *alertInstance) checkDiskUsageAlert(serverId int64, r *alertRule) (bool, *time.Time) {
	calculator := func(statuses []*_type.ServerStatusType, startTime time.Time) (float64, int) {
		var sum float64
		var count int
		for _, item := range statuses {
			if item.Time.After(startTime) {
				sum += (item.DiskUsedGB / item.DiskTotalGB) * 100 // Convert to percentage
				count++
			}
		}
		return sum, count
	}

	return a.checkMetricAlert(
		serverId,
		r,
		calculator,
		"Disk usage exceeded threshold",
		func(value float64, duration int, threshold int) string {
			return fmt.Sprintf("%.2f%% Disk usage over the last %d minutes exceeded the threshold of %d%%.",
				value, duration, threshold)
		},
	)
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
			return fmt.Sprintf("%.1f Read IOPS over the last %d minutes exceeded the threshold of %d IOPS.",
				value, duration, threshold)
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
			return fmt.Sprintf("%.1f Write IOPS over the last %d minutes exceeded the threshold of %d IOPS.",
				value, duration, threshold)
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
			return fmt.Sprintf("%.2f Mbps Bandwidth usage over the last %d minutes exceeded the threshold of %d Mbps.",
				value, duration, threshold)
		},
	)
}
