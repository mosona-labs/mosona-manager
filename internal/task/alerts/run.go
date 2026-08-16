package alerttasks

import (
	"context"
	"log"
	"mosona-manager/internal/db"
	"time"
)

const alertUpdateTimeout = 15 * time.Second

func Run() {
	rulesMap, serverMap, expiryMap, err := allServerAlerts()
	if err != nil {
		log.Printf("failed to get rules: %v", err)
		return
	}
	if len(rulesMap) == 0 {
		return
	}
	teamIds := make([]int64, 0, len(rulesMap))
	for teamId := range rulesMap {
		teamIds = append(teamIds, teamId)
	}

	notificationsMap, err := teamNotificationsByIds(teamIds)
	if err != nil {
		log.Printf("failed to get notifications: %v", err)
		return
	}

	for _, teamId := range teamIds {
		notifications := notificationsMap[teamId]
		rules := rulesMap[teamId]
		if len(rules) == 0 {
			continue
		}

		var updateQueue []alertRuleUpdate

		for serverId, serverRules := range rules {
			maxTime := 0
			for _, rule := range serverRules {
				if rule.ForDuration > maxTime {
					maxTime = rule.ForDuration
				}
			}

			now := time.Now()
			statuses, err := statusWindow(serverId, maxTime, now)
			if err != nil {
				log.Printf("failed to get status window for server %d: %v", serverId, err)
				continue
			}

			alert := &alertInstance{
				serverMap:     &serverMap,
				expiryMap:     expiryMap,
				notifications: notifications,
				statuses:      statuses,
			}

			for rule, serverRule := range serverRules {
				r := &alertRule{
					id:           serverRule.ID,
					startTime:    now.Add(-time.Duration(serverRule.ForDuration) * time.Minute),
					threshold:    serverRule.Threshold,
					forDuration:  serverRule.ForDuration,
					lastStatus:   serverRule.LastStatus,
					lastNotifyAt: serverRule.LastNotifyAt,
				}

				var ls *bool
				var ln *time.Time

				switch rule {
				case "status":
					ls, ln = alert.checkStatusAlert(serverId, r)
				case "cpu_usage":
					ls, ln = alert.checkCPUUsageAlert(serverId, r)
				case "memory_usage":
					ls, ln = alert.checkMemoryUsageAlert(serverId, r)
				case "disk_usage":
					ls, ln = alert.checkDiskUsageAlert(serverId, r)
				case "read_iops":
					ls, ln = alert.checkReadIOPSAlert(serverId, r)
				case "write_iops":
					ls, ln = alert.checkWriteIOPSAlert(serverId, r)
				case "bandwidth":
					ls, ln = alert.checkBandwidthAlert(serverId, r)
				case "expiry_reminder":
					ls, ln = alert.checkExpiryReminderAlert(serverId, r)
				default:
					continue
				}

				if alertStateEqual(serverRule.LastStatus, ls, serverRule.LastNotifyAt, ln) {
					continue
				}
				updateQueue = append(updateQueue, alertRuleUpdate{
					id:           r.id,
					lastStatus:   ls,
					lastNotifyAt: ln,
				})
			}
		}

		if err = updateRuleStatus(updateQueue); err != nil {
			log.Printf("failed to update rule status for team %d: %v", teamId, err)
		}
	}
}

func updateRuleStatus(queue []alertRuleUpdate) error {
	if len(queue) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), alertUpdateTimeout)
	defer cancel()
	tx, err := db.Db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `UPDATE server_alerts
		SET last_status = $1, last_notify_at = $2
		WHERE id = $3
		  AND (last_status IS DISTINCT FROM $1 OR last_notify_at IS DISTINCT FROM $2)`)
	if err != nil {
		return err
	}
	defer func() {
		_ = stmt.Close()
	}()

	for _, u := range queue {
		if _, err = stmt.ExecContext(ctx, u.lastStatus, u.lastNotifyAt, u.id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func alertStateEqual(oldStatus, newStatus *bool, oldNotifyAt, newNotifyAt *time.Time) bool {
	if oldStatus == nil || newStatus == nil {
		if oldStatus != nil || newStatus != nil {
			return false
		}
	} else if *oldStatus != *newStatus {
		return false
	}

	if oldNotifyAt == nil || newNotifyAt == nil {
		return oldNotifyAt == nil && newNotifyAt == nil
	}
	return oldNotifyAt.Equal(*newNotifyAt)
}
