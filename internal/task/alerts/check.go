package alerttasks

import (
	"fmt"
	"time"
)

type statusNotification int

const (
	statusNotificationNone statusNotification = iota
	statusNotificationDown
	statusNotificationUp
)

func statusNotificationFor(lastStatus *bool, currentStatus bool) statusNotification {
	if lastStatus == nil {
		if !currentStatus {
			return statusNotificationDown
		}
		return statusNotificationNone
	}

	if *lastStatus == currentStatus {
		return statusNotificationNone
	}
	if currentStatus {
		return statusNotificationUp
	}
	return statusNotificationDown
}

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

func statusPtr(status bool) *bool {
	return &status
}

func (a *alertInstance) checkMetricAlert(
	serverId int64,
	r *alertRule,
	alertTitle string,
	messageFormatter func(value float64, duration int, threshold int) string,
) (*bool, *time.Time) {
	if !a.observation.present {
		return statusPtr(false), r.lastNotifyAt
	}

	avgValue := a.observation.value
	currentStatus := avgValue >= float64(r.threshold)

	if shouldNotify(currentStatus, r) {
		delivery := a.notifyAll(serverId, alertTitle, messageFormatter(avgValue, r.forDuration, r.threshold))
		if delivery.delivered {
			now := time.Now()
			return statusPtr(currentStatus), &now
		}
		if delivery.attempted {
			return r.lastStatus, r.lastNotifyAt
		}
	}

	return statusPtr(currentStatus), r.lastNotifyAt
}

func (a *alertInstance) checkStatusAlert(serverId int64, r *alertRule) (*bool, *time.Time) {
	currentStatus := a.observation.present

	notification := statusNotificationFor(r.lastStatus, currentStatus)
	if notification != statusNotificationNone {
		var delivery notificationDelivery
		if notification == statusNotificationUp {
			delivery = a.notifyAll(serverId, "Server Up", "The server is now UP.")
		} else {
			delivery = a.notifyAll(
				serverId,
				"Server Down",
				fmt.Sprintf("No response for %d minutes.", r.forDuration),
			)
		}
		if delivery.delivered {
			now := time.Now()
			return statusPtr(currentStatus), &now
		}
		if delivery.attempted {
			return r.lastStatus, r.lastNotifyAt
		}
	}

	return statusPtr(currentStatus), r.lastNotifyAt
}

func (a *alertInstance) checkCPUUsageAlert(serverId int64, r *alertRule) (*bool, *time.Time) {
	return a.checkMetricAlert(
		serverId,
		r,
		"CPU usage exceeded threshold",
		func(value float64, duration int, threshold int) string {
			return fmt.Sprintf("The %d-minutes average CPU usage reached %.2f%%, exceeding the configured threshold of %d%%",
				duration, value, threshold)
		},
	)
}

func (a *alertInstance) checkMemoryUsageAlert(serverId int64, r *alertRule) (*bool, *time.Time) {
	return a.checkMetricAlert(
		serverId,
		r,
		"Memory usage exceeded threshold",
		func(value float64, duration int, threshold int) string {
			return fmt.Sprintf("The %d-minutes average Memory usage reached %.2f%%, exceeding the configured threshold of %d%%",
				duration, value, threshold)
		},
	)
}

func (a *alertInstance) checkDiskUsageAlert(serverId int64, r *alertRule) (*bool, *time.Time) {
	if !a.observation.present {
		return statusPtr(false), r.lastNotifyAt
	}
	avgValue := a.observation.value
	mountPoint := a.observation.mountPoint
	currentStatus := avgValue >= float64(r.threshold)

	if shouldNotify(currentStatus, r) {
		delivery := a.notifyAll(
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
		if delivery.delivered {
			now := time.Now()
			return statusPtr(currentStatus), &now
		}
		if delivery.attempted {
			return r.lastStatus, r.lastNotifyAt
		}
	}

	return statusPtr(currentStatus), r.lastNotifyAt
}

func (a *alertInstance) checkExpiryReminderAlert(serverId int64, r *alertRule) (*bool, *time.Time) {
	serverExpiry, ok := a.expiryMap[serverId]
	if !ok || serverExpiry.EndTime == nil || serverExpiry.AutoRenew {
		return statusPtr(false), r.lastNotifyAt
	}

	remaining := time.Until(*serverExpiry.EndTime)
	currentStatus := remaining > 0 && remaining <= time.Duration(r.threshold)*24*time.Hour
	if !currentStatus {
		return statusPtr(false), r.lastNotifyAt
	}

	if r.lastStatus != nil && *r.lastStatus {
		return statusPtr(true), r.lastNotifyAt
	}

	daysLeft := int((remaining + 24*time.Hour - 1) / (24 * time.Hour))
	if daysLeft < 1 {
		daysLeft = 1
	}
	delivery := a.notifyAll(
		serverId,
		"Server expiry reminder",
		fmt.Sprintf(
			"This server is not set to auto-renew and will expire in %d day(s) on %s.",
			daysLeft,
			serverExpiry.EndTime.Format("2006-01-02 15:04 MST"),
		),
	)
	if delivery.delivered {
		now := time.Now()
		return statusPtr(true), &now
	}
	if delivery.attempted {
		return r.lastStatus, r.lastNotifyAt
	}
	return statusPtr(true), r.lastNotifyAt
}

func (a *alertInstance) checkReadIOPSAlert(serverId int64, r *alertRule) (*bool, *time.Time) {
	return a.checkMetricAlert(
		serverId,
		r,
		"Read IOPS exceeded threshold",
		func(value float64, duration int, threshold int) string {
			return fmt.Sprintf("The %d-minutes average Read IOPS reached %.1f, exceeding the configured threshold of %d IOPS",
				duration, value, threshold)
		},
	)
}

func (a *alertInstance) checkWriteIOPSAlert(serverId int64, r *alertRule) (*bool, *time.Time) {
	return a.checkMetricAlert(
		serverId,
		r,
		"Write IOPS exceeded threshold",
		func(value float64, duration int, threshold int) string {
			return fmt.Sprintf("The %d-minutes average Write IOPS reached %.1f, exceeding the configured threshold of %d IOPS",
				duration, value, threshold)
		},
	)
}

func (a *alertInstance) checkBandwidthAlert(serverId int64, r *alertRule) (*bool, *time.Time) {
	return a.checkMetricAlert(
		serverId,
		r,
		"Bandwidth usage exceeded threshold",
		func(value float64, duration int, threshold int) string {
			return fmt.Sprintf("The %d-minutes average Bandwidth usage reached %.2f Mbps, exceeding the configured threshold of %d Mbps",
				duration, value, threshold)
		},
	)
}
