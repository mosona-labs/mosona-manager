package db

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
)

var allowedAlertItems = map[string]bool{
	"status":       true,
	"cpu_usage":    true,
	"memory_usage": true,
	"disk_usage":   true,
	"read_iops":    true,
	"write_iops":   true,
	"bandwidth":    true,
}

var ErrAlertNotFound = errors.New("alert not found")

func UpsertServerAlert(teamId, serverId int64, item string, threshold, forDuration int) error {
	if !allowedAlertItems[item] {
		return ErrAlertNotFound
	}

	_, err := Db.Exec(`
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
	if !allowedAlertItems[item] {
		return 0, ErrAlertNotFound
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
	if !allowedAlertItems[item] {
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
	if !allowedAlertItems[item] {
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
		var id int64
		var item string
		var threshold, forDuration int

		if err = rows.Scan(&id, &item, &threshold, &forDuration); err != nil {
			return nil, err
		}

		alerts[item] = _type.ServerAlert{
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
