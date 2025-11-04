package db

import "mosona-manager/_type"

func GetTeamById(id int64) (_type.Team, error) {
	var team _type.Team
	if err := Db.Get(
		&team,
		"SELECT id, name, description, color, image, max_server, max_alert, max_member, updated_at, created_at FROM teams WHERE id = $1",
		id,
	); err != nil {
		return _type.Team{}, err
	}

	return team, nil
}

func CreateTeam(
	name string,
	description string,
	avatarColor string,
	avatarUrl string,
	members []_type.TeamUsersRole,
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

	// Add Members
	for _, member := range members {
		if _, err = tx.Exec(`INSERT INTO m_team_user (user_id, team_id, role) VALUES ($1, $2, $3)`,
			member.ID, teamId, member.Role,
		); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
	}

	// Add Default Category
	if _, err = tx.Exec(`INSERT INTO categories (team, name) VALUES ($1, $2)`, teamId, "Default"); err != nil {
		_ = tx.Rollback()
		return 0, err
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

func GetTeamMembers(teamId int64) ([]_type.TeamUsersRole, error) {
	var members = make([]_type.TeamUsersRole, 0)
	if err := Db.Select(&members, `
		SELECT user_id as id, u.username, u.email, u.is_admin, u.updated_at, u.created_at, role
		FROM m_team_user
		LEFT JOIN users u ON m_team_user.user_id = u.id
		WHERE team_id = $1
	`, teamId); err != nil {
		return nil, err
	}

	return members, nil
}
