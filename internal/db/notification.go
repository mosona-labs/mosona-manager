package db

import "mosona-manager/internal/_type"

func GetNotificationsByTeamId(teamID int64) ([]_type.TeamNotification, error) {
	var notifications = make([]_type.TeamNotification, 0)
	if err := Db.Select(
		&notifications,
		"SELECT module, target FROM teams_notifications WHERE team_id=$1",
		teamID,
	); err != nil {
		return nil, err
	}
	return notifications, nil
}

func UpdateNotificationsByTeamId(teamID int64, notifications []_type.TeamNotification) error {
	tx, err := Db.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM teams_notifications WHERE team_id=$1", teamID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, n := range notifications {
		_, err = tx.Exec(
			"INSERT INTO teams_notifications (team_id, module, target) VALUES ($1, $2, $3)",
			teamID, n.Module, n.Target,
		)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
