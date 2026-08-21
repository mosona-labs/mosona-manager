package alerttasks

import (
	"context"
	"fmt"
	"log"
	"mosona-manager/internal/db"
	"sort"
	"strings"
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
	now := time.Now()
	loadCtx, cancelLoad := context.WithTimeout(context.Background(), alertLoadTimeout)
	observations := loadAlertObservations(loadCtx, rulesMap, now, executeAlertQuery)
	cancelLoad()
	skippedRules := make(map[string]int)
	totalSkippedRules := 0

	for _, teamId := range teamIds {
		notifications := notificationsMap[teamId]
		rules := rulesMap[teamId]
		if len(rules) == 0 {
			continue
		}

		var updateQueue []alertRuleUpdate

		for serverId, serverRules := range rules {
			alert := &alertInstance{
				serverMap:     &serverMap,
				expiryMap:     expiryMap,
				notifications: notifications,
			}

			for rule, serverRule := range serverRules {
				if !alert.setObservationForRule(observations, serverId, rule) {
					skippedRules[rule]++
					totalSkippedRules++
					continue
				}
				r := &alertRule{
					id:           serverRule.ID,
					threshold:    serverRule.Threshold,
					forDuration:  serverRule.ForDuration,
					lastStatus:   serverRule.LastStatus,
					lastNotifyAt: serverRule.LastNotifyAt,
				}

				var ls *bool
				var ln *time.Time

				switch rule {
				case alertItemStatus:
					ls, ln = alert.checkStatusAlert(serverId, r)
				case alertItemCPU:
					ls, ln = alert.checkCPUUsageAlert(serverId, r)
				case alertItemMemory:
					ls, ln = alert.checkMemoryUsageAlert(serverId, r)
				case alertItemDisk:
					ls, ln = alert.checkDiskUsageAlert(serverId, r)
				case alertItemReadIOPS:
					ls, ln = alert.checkReadIOPSAlert(serverId, r)
				case alertItemWriteIOPS:
					ls, ln = alert.checkWriteIOPSAlert(serverId, r)
				case alertItemBandwidth:
					ls, ln = alert.checkBandwidthAlert(serverId, r)
				case alertItemExpiry:
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
	if totalSkippedRules > 0 {
		log.Print(skippedAlertRulesMessage(totalSkippedRules, skippedRules, observations))
	}
}

func (a *alertInstance) setObservationForRule(
	observations *alertObservationSet,
	serverID int64,
	item string,
) bool {
	a.observation = alertObservation{}
	if item == alertItemExpiry {
		return true
	}
	observation, loaded := observations.get(serverID, item)
	if loaded {
		a.observation = observation
	}
	return loaded
}

func skippedAlertRulesMessage(total int, skippedRules map[string]int, observations *alertObservationSet) string {
	items := make([]string, 0, len(skippedRules))
	for item := range skippedRules {
		items = append(items, item)
	}
	sort.Strings(items)
	counts := make([]string, 0, len(items))
	for _, item := range items {
		counts = append(counts, fmt.Sprintf("%s=%d", item, skippedRules[item]))
	}
	return fmt.Sprintf(
		"alert evaluation skipped %d rules with unavailable observations (%s; query_failures=%d invalid_durations=%d load_stopped=%t)",
		total,
		strings.Join(counts, ", "),
		observations.queryFailures,
		observations.invalidDurations,
		observations.loadStopped,
	)
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
