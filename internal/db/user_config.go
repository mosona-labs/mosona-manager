package db

import (
	"database/sql"
	"errors"
)

func SetUserActiveTeam(uid, tid int64) error {
	if tid == 0 {
		_, err := Db.Exec("DELETE FROM users_config WHERE uid = $1", uid)
		return err
	}

	var activeTeam int64
	return Db.QueryRow(`
		INSERT INTO users_config (uid, active_team)
		SELECT $1, $2
		FROM m_team_user
		WHERE user_id = $1 AND team_id = $2
		ON CONFLICT (uid) DO UPDATE SET active_team = EXCLUDED.active_team
		RETURNING active_team
	`, uid, tid).Scan(&activeTeam)
}

func GetUserActiveTeam(uid int64) (int64, error) {
	var tid int64
	err := Db.Get(&tid, `
		SELECT uc.active_team
		FROM users_config AS uc
		JOIN m_team_user AS mtu
		  ON mtu.team_id = uc.active_team AND mtu.user_id = uc.uid
		WHERE uc.uid = $1
	`, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	return tid, nil
}

func ClearUserActiveTeam(uid, staleTid int64) error {
	_, err := Db.Exec(
		"DELETE FROM users_config WHERE uid = $1 AND active_team = $2",
		uid, staleTid,
	)
	return err
}
