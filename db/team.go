package db

import "mosona-manager/_type"

func GetTeamById(id int64) (_type.Team, error) {
	var team _type.Team
	if err := Db.Get(&team, "SELECT id, name, description, color, image, max_server, max_alert, max_member, updated_at, created_at FROM teams WHERE id = $1", id); err != nil {
		return _type.Team{}, err
	}

	return team, nil
}

func CreateTeam(
	name string,
	description string,
	avatarColor string,
	avatarUrl string,
	members []int64,
	maxServer int,
	maxAlert int,
	maxMember int,
	planId int64,
	ownerId int64,
) (int64, error) {
	tx, err := Db.Beginx()
	if err != nil {
		return 0, err
	}

	var teamId int64
	if err = tx.QueryRow(
		`INSERT INTO teams (name, description, color, image, max_server, max_alert, max_member, plan_id, owner_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		name, description, avatarColor, avatarUrl, maxServer, maxAlert, maxMember, planId, ownerId,
	).Scan(&teamId); err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	for _, memberId := range members {
		if _, err = tx.Exec(`INSERT INTO m_team_user (user_id, team_id)
			 VALUES ($1, $2)`,
			memberId, teamId,
		); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
	}

	return teamId, tx.Commit()
}

func GetTeamsByUserId(uid int64) ([]_type.Team, error) {
	var teams = make([]_type.Team, 0)
	if err := Db.Select(&teams, `
		SELECT t.id, t.name, t.description, t.color, t.image, t.max_server, t.max_alert, t.max_member, t.updated_at, t.created_at
		FROM teams t
		JOIN m_team_user mtu ON t.id = mtu.team_id
		WHERE mtu.user_id = $1
	`, uid); err != nil {
		return nil, err
	}

	return teams, nil
}
