package db

import (
	"database/sql"
	"errors"
)

func SetUserActiveTeam(uid, tid int64) error {
	var exists int
	err := Db.Get(&exists, "SELECT 1 FROM m_team_user WHERE team_id = $1 AND user_id = $2", tid, uid)
	if err != nil {
		return err
	}

	_, err = Db.Exec(`
		INSERT INTO users_config (uid, active_team) VALUES ($1, $2)
		ON CONFLICT (uid) DO UPDATE SET active_team = EXCLUDED.active_team
	`, uid, tid)
	return err
}

func GetUserActiveTeam(uid int64) (int64, error) {
	var tid int64
	err := Db.Get(&tid, "SELECT active_team FROM users_config WHERE uid = $1", uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	return tid, nil
}
