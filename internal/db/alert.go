package db

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
)

type alertItemRule struct {
	label           string
	description     string
	threshold       _type.AlertFieldConfig
	forDuration     _type.AlertFieldConfig
	notifyOnce      bool
	normalizeConfig func(threshold, forDuration int) (int, int, error)
}

var alertItemRules = map[string]alertItemRule{
	"status": {
		label:       "Status",
		description: "Trigger when the server has no status data during the configured lookback window.",
		threshold:   _type.AlertFieldConfig{Enabled: false},
		forDuration: _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 1440, Default: 10, Unit: "minute"},
	},
	"cpu_usage": {
		label:       "CPU Usage",
		description: "Trigger when average CPU usage across the window exceeds the threshold.",
		threshold:   _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 100, Default: 80, Unit: "percent"},
		forDuration: _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 1440, Default: 10, Unit: "minute"},
	},
	"memory_usage": {
		label:       "Memory Usage",
		description: "Trigger when average memory usage across the window exceeds the threshold.",
		threshold:   _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 100, Default: 80, Unit: "percent"},
		forDuration: _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 1440, Default: 10, Unit: "minute"},
	},
	"disk_usage": {
		label:       "Disk Usage",
		description: "Trigger when any partition average usage across the window exceeds the threshold.",
		threshold:   _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 100, Default: 80, Unit: "percent"},
		forDuration: _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 1440, Default: 10, Unit: "minute"},
	},
	"read_iops": {
		label:       "Read IOPS",
		description: "Trigger when average read IOPS across the window exceeds the threshold.",
		threshold:   _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 1000000, Default: 1000, Unit: "iops"},
		forDuration: _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 1440, Default: 10, Unit: "minute"},
	},
	"write_iops": {
		label:       "Write IOPS",
		description: "Trigger when average write IOPS across the window exceeds the threshold.",
		threshold:   _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 1000000, Default: 1000, Unit: "iops"},
		forDuration: _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 1440, Default: 10, Unit: "minute"},
	},
	"bandwidth": {
		label:       "Bandwidth",
		description: "Trigger when average bandwidth usage across the window exceeds the threshold.",
		threshold:   _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 1000000, Default: 100, Unit: "mbps"},
		forDuration: _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 1440, Default: 10, Unit: "minute"},
	},
	"expiry_reminder": {
		label:       "Expiry Reminder",
		description: "Trigger once when a non-auto-renew server enters the configured number of days before expiration.",
		threshold:   _type.AlertFieldConfig{Enabled: true, Min: 1, Max: 7, Default: 3, Unit: "day"},
		forDuration: _type.AlertFieldConfig{Enabled: false},
		notifyOnce:  true,
		normalizeConfig: func(threshold, _ int) (int, int, error) {
			if threshold < 1 || threshold > 7 {
				return 0, 0, ErrAlertInvalidConfig
			}
			return threshold, 0, nil
		},
	},
}

var ErrAlertNotFound = errors.New("alert not found")
var ErrAlertInvalidConfig = errors.New("alert invalid config")

func normalizeAlertConfig(item string, threshold, forDuration int) (int, int, error) {
	rule, ok := alertItemRules[item]
	if !ok {
		return 0, 0, ErrAlertNotFound
	}
	if rule.normalizeConfig != nil {
		return rule.normalizeConfig(threshold, forDuration)
	}
	return threshold, forDuration, nil
}

func AlertItemConfigs() []_type.AlertItemConfig {
	items := []string{
		"status",
		"cpu_usage",
		"memory_usage",
		"disk_usage",
		"read_iops",
		"write_iops",
		"bandwidth",
		"expiry_reminder",
	}
	configs := make([]_type.AlertItemConfig, 0, len(items))
	for _, item := range items {
		rule := alertItemRules[item]
		configs = append(configs, _type.AlertItemConfig{
			Item:        item,
			Label:       rule.label,
			Description: rule.description,
			Threshold:   rule.threshold,
			ForDuration: rule.forDuration,
			NotifyOnce:  rule.notifyOnce,
		})
	}
	return configs
}

func UpsertServerAlert(teamId, serverId int64, item string, threshold, forDuration int) error {
	var err error
	threshold, forDuration, err = normalizeAlertConfig(item, threshold, forDuration)
	if err != nil {
		return err
	}

	_, err = Db.Exec(`
		INSERT INTO server_alerts (server_id, item, threshold, for_duration)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (server_id, item) DO UPDATE
		SET threshold = EXCLUDED.threshold,
		    for_duration = EXCLUDED.for_duration
		WHERE EXISTS (
			SELECT 1 FROM servers WHERE id = $1 AND team_id = $5
		)
	`, serverId, item, threshold, forDuration, teamId)
	if err != nil {
		return err
	}
	return nil
}

func UpsertTeamAlert(teamId int64, item string, threshold, forDuration int, override bool) (int64, error) {
	var err error
	threshold, forDuration, err = normalizeAlertConfig(item, threshold, forDuration)
	if err != nil {
		return 0, err
	}

	tx, err := Db.Begin()
	if err != nil {
		return 0, err
	}

	_, err = tx.Exec(`
  INSERT INTO team_alerts (team_id, item, threshold, for_duration)
  VALUES ($1, $2, $3, $4)
  ON CONFLICT (team_id, item) DO UPDATE
  SET threshold = EXCLUDED.threshold,
      for_duration = EXCLUDED.for_duration
 `, teamId, item, threshold, forDuration)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	rows, err := tx.Query("SELECT id FROM servers WHERE team_id = $1", teamId)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	var serverIds []int64
	for rows.Next() {
		var serverId int64
		if err = rows.Scan(&serverId); err != nil {
			_ = rows.Close()
			_ = tx.Rollback()
			return 0, err
		}
		serverIds = append(serverIds, serverId)
	}
	_ = rows.Close()

	if err = rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	var affectedCount int64 = 0
	for _, serverId := range serverIds {
		var result sql.Result
		if override {
			result, err = tx.Exec(`
    INSERT INTO server_alerts (server_id, item, threshold, for_duration)
    VALUES ($1, $2, $3, $4)
    ON CONFLICT (server_id, item) DO UPDATE
    SET threshold = EXCLUDED.threshold,
        for_duration = EXCLUDED.for_duration
   `, serverId, item, threshold, forDuration)
		} else {
			result, err = tx.Exec(`
    INSERT INTO server_alerts (server_id, item, threshold, for_duration)
    VALUES ($1, $2, $3, $4)
    ON CONFLICT (server_id, item) DO NOTHING
   `, serverId, item, threshold, forDuration)
		}
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		affected, _ := result.RowsAffected()
		affectedCount += affected
	}

	return affectedCount, tx.Commit()
}

func DeleteServerAlert(teamId, serverId int64, item string) error {
	if _, ok := alertItemRules[item]; !ok {
		return ErrAlertNotFound
	}

	_, err := Db.Exec(`
		DELETE FROM server_alerts
		WHERE server_id = $1 AND item = $2
		AND EXISTS (
			SELECT 1 FROM servers WHERE id = $1 AND team_id = $3
		)
	`, serverId, item, teamId)
	return err
}

func DeleteTeamAlert(teamId int64, item string, override bool) (int64, error) {
	if _, ok := alertItemRules[item]; !ok {
		return 0, ErrAlertNotFound
	}

	tx, err := Db.Begin()
	if err != nil {
		return 0, err
	}

	var data _type.ServerAlert
	if !override {
		if err = tx.QueryRow(
			"SELECT threshold, for_duration FROM team_alerts WHERE team_id = $1 AND item = $2",
			teamId, item,
		).Scan(&data.Threshold, &data.ForDuration); err != nil {
			return 0, err
		}
	}

	_, err = tx.Exec(`
		DELETE FROM team_alerts
		WHERE team_id = $1 AND item = $2
	`, teamId, item)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	var result sql.Result
	if override {
		result, err = tx.Exec(`
			DELETE FROM server_alerts
			WHERE item = $1 AND server_id IN (
				SELECT id FROM servers WHERE team_id = $2
			)
		`, item, teamId)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
	} else {
		result, err = tx.Exec(`
			DELETE FROM server_alerts
			WHERE item = $1 AND threshold = $2 AND for_duration = $3
			AND server_id IN (
				SELECT id FROM servers WHERE team_id = $4
			)
		`, item, data.Threshold, data.ForDuration, teamId)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
	}
	affectedCount, _ := result.RowsAffected()

	return affectedCount, tx.Commit()
}

func GetServerAlertsByTeamId(teamId int64) (map[int64]map[string]_type.ServerAlert, error) {
	alerts := make(map[int64]map[string]_type.ServerAlert)

	rows, err := Db.Query(`
		SELECT sa.id, sa.server_id, sa.item, sa.threshold, sa.for_duration
		FROM server_alerts sa
		JOIN servers s ON sa.server_id = s.id
		WHERE s.team_id = $1
	`, teamId)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var id, serverId int64
		var item string
		var threshold, forDuration int

		if err = rows.Scan(&id, &serverId, &item, &threshold, &forDuration); err != nil {
			return nil, err
		}

		if _, exists := alerts[serverId]; !exists {
			alerts[serverId] = make(map[string]_type.ServerAlert)
		}
		alerts[serverId][item] = _type.ServerAlert{
			ID:          id,
			Item:        item,
			Threshold:   threshold,
			ForDuration: forDuration,
		}
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return alerts, nil
}

func GetTeamAlertsByTeamId(teamId int64) (map[string]_type.ServerAlert, error) {
	alerts := make(map[string]_type.ServerAlert)

	rows, err := Db.Query(`
		SELECT id, item, threshold, for_duration
		FROM team_alerts
		WHERE team_id = $1
	`, teamId)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var data _type.ServerAlert
		if err = rows.Scan(&data.ID, &data.Item, &data.Threshold, &data.ForDuration); err != nil {
			return nil, err
		}

		alerts[data.Item] = data
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return alerts, nil
}

func GetTeamAlertsByTeamIdTx(tx *sql.Tx, teamId int64) (map[string]_type.ServerAlert, error) {
	alerts := make(map[string]_type.ServerAlert)

	rows, err := tx.Query(`
		SELECT id, item, threshold, for_duration
		FROM team_alerts
		WHERE team_id = $1
	`, teamId)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var data _type.ServerAlert
		if err = rows.Scan(&data.ID, &data.Item, &data.Threshold, &data.ForDuration); err != nil {
			return nil, err
		}

		alerts[data.Item] = data
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return alerts, nil
}
