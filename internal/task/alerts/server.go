package alerttasks

import (
	"database/sql"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"time"

	"github.com/jmoiron/sqlx"
)

func teamNotificationsByIds(teamIds []int64) (map[int64][]*_type.TeamNotification, error) {
	if len(teamIds) == 0 {
		return make(map[int64][]*_type.TeamNotification), nil
	}

	var notifications []struct {
		TeamId int64  `db:"team_id"`
		Module string `db:"module"`
		Target string `db:"target"`
	}

	query, args, err := sqlx.In(
		"SELECT team_id, module, target FROM teams_notifications WHERE team_id IN (?)",
		teamIds,
	)
	if err != nil {
		return nil, err
	}
	query = db.Db.Rebind(query)

	if err := db.Db.Select(&notifications, query, args...); err != nil {
		return nil, err
	}

	// Pre-allocate map capacity
	result := make(map[int64][]*_type.TeamNotification, len(teamIds))
	for _, n := range notifications {
		result[n.TeamId] = append(result[n.TeamId], &_type.TeamNotification{
			Module: n.Module,
			Target: n.Target,
		})
	}
	return result, nil
}

func allServerAlerts() (map[int64]map[int64]map[string]_type.ServerAlert, map[int64]string, map[int64]serverExpiryInfo, error) {
	type alertRow struct {
		TeamId       int64        `db:"team_id"`
		ID           int64        `db:"id"`
		ServerID     int64        `db:"server_id"`
		ServerName   string       `db:"name"`
		Item         string       `db:"item"`
		Threshold    int          `db:"threshold"`
		ForDuration  int          `db:"for_duration"`
		LastStatus   sql.NullBool `db:"last_status"`
		LastNotifyAt sql.NullTime `db:"last_notify_at"`
		EndTime      sql.NullTime `db:"end_time"`
		AutoRenew    sql.NullBool `db:"auto_renew"`
	}

	var rows []alertRow
	err := db.Db.Select(&rows, `
		SELECT s.team_id, a.id, a.server_id, s.name, a.item, a.threshold, a.for_duration, a.last_status, a.last_notify_at, i.end_time, COALESCE(i.auto_renew, false) AS auto_renew
		FROM server_alerts a
		JOIN servers s ON a.server_id = s.id
		LEFT JOIN server_info i ON a.server_id = i.sid
	`)
	if err != nil {
		return nil, nil, nil, err
	}

	// Pre-allocate map capacity
	alerts := make(map[int64]map[int64]map[string]_type.ServerAlert)
	serverMap := make(map[int64]string)
	expiryMap := make(map[int64]serverExpiryInfo)

	// If no alerts, return empty map
	if len(rows) == 0 {
		return alerts, serverMap, expiryMap, nil
	}

	for _, row := range rows {
		if _, exists := alerts[row.TeamId]; !exists {
			alerts[row.TeamId] = make(map[int64]map[string]_type.ServerAlert)
		}
		if _, exists := alerts[row.TeamId][row.ServerID]; !exists {
			alerts[row.TeamId][row.ServerID] = make(map[string]_type.ServerAlert)
		}

		var lastStatus *bool
		if row.LastStatus.Valid {
			lastStatus = &row.LastStatus.Bool
		}
		var lastNotifyAt *time.Time
		if row.LastNotifyAt.Valid {
			lastNotifyAt = &row.LastNotifyAt.Time
		}

		alerts[row.TeamId][row.ServerID][row.Item] = _type.ServerAlert{
			ID:           row.ID,
			Item:         row.Item,
			Threshold:    row.Threshold,
			ForDuration:  row.ForDuration,
			LastStatus:   lastStatus,
			LastNotifyAt: lastNotifyAt,
		}
		serverMap[row.ServerID] = row.ServerName
		expiryInfo := serverExpiryInfo{
			AutoRenew: row.AutoRenew.Valid && row.AutoRenew.Bool,
		}
		if row.EndTime.Valid {
			endTime := row.EndTime.Time
			expiryInfo.EndTime = &endTime
		}
		expiryMap[row.ServerID] = expiryInfo
	}

	return alerts, serverMap, expiryMap, nil
}
