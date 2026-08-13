package db

import (
	"context"
	"database/sql"
	"mosona-manager/internal/_type"
)

func GetTeamRole(ctx context.Context, userID, teamID int64) (int, error) {
	var role int
	err := Db.QueryRowContext(ctx,
		"SELECT role FROM m_team_user WHERE user_id = $1 AND team_id = $2",
		userID, teamID,
	).Scan(&role)
	return role, err
}

func LeaveTeam(ctx context.Context, userID, teamID int64) error {
	tx, err := Db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var ownerID int64
	if err = tx.QueryRowContext(ctx,
		"SELECT owner_id FROM teams WHERE id = $1 FOR UPDATE",
		teamID,
	).Scan(&ownerID); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx,
		"SELECT user_id FROM m_team_user WHERE team_id = $1 ORDER BY user_id FOR UPDATE",
		teamID,
	)
	if err != nil {
		return err
	}
	members := make([]int64, 0)
	isMember := false
	for rows.Next() {
		var memberID int64
		if err = rows.Scan(&memberID); err != nil {
			_ = rows.Close()
			return err
		}
		members = append(members, memberID)
		isMember = isMember || memberID == userID
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if !isMember {
		return sql.ErrNoRows
	}

	if ownerID == userID && len(members) == 1 {
		if _, err = tx.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", teamID); err != nil {
			return err
		}
		return tx.Commit()
	}

	if ownerID == userID {
		var newOwnerID int64
		for _, memberID := range members {
			if memberID != userID {
				newOwnerID = memberID
				break
			}
		}
		if _, err = tx.ExecContext(ctx,
			"UPDATE m_team_user SET role = 0 WHERE team_id = $1 AND user_id = $2",
			teamID, newOwnerID,
		); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx,
			"UPDATE teams SET owner_id = $1 WHERE id = $2",
			newOwnerID, teamID,
		); err != nil {
			return err
		}
	}

	if _, err = tx.ExecContext(ctx,
		"DELETE FROM users_config WHERE uid = $1 AND active_team = $2",
		userID, teamID,
	); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx,
		"DELETE FROM m_team_user WHERE user_id = $1 AND team_id = $2",
		userID, teamID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func GetTeamById(id int64) (_type.Team, error) {
	var team _type.Team
	if err := Db.Get(
		&team,
		"SELECT id, owner_id, name, description, color, image, updated_at, created_at FROM teams WHERE id = $1",
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
	ownerId int64,
) (int64, error) {
	tx, err := Db.Beginx()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var teamId int64
	if err = tx.QueryRow(
		`INSERT INTO teams (name, description, color, image, owner_id)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		name, description, avatarColor, avatarUrl, ownerId,
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
		SELECT t.id, t.owner_id, t.name, t.description, t.color, t.image, t.updated_at, t.created_at
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

func IsTeamOwner(teamId int64, userId int64) (bool, error) {
	var ownerId int64
	if err := Db.Get(&ownerId, `SELECT owner_id FROM teams WHERE id = $1`, teamId); err != nil {
		return false, err
	}

	return ownerId == userId, nil
}

func TransferTeamOwnership(teamId int64, newOwnerId int64) error {
	tx, err := Db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var teamExists int
	if err = tx.QueryRow(
		"SELECT 1 FROM teams WHERE id = $1 FOR UPDATE",
		teamId,
	).Scan(&teamExists); err != nil {
		return err
	}
	if _, err = tx.Exec(`
		INSERT INTO m_team_user (team_id, user_id, role) VALUES ($1, $2, 0)
		ON CONFLICT (team_id, user_id) DO UPDATE SET role = 0
		WHERE m_team_user.role <> 0
	`, teamId, newOwnerId); err != nil {
		return err
	}
	if _, err = tx.Exec(
		"UPDATE teams SET owner_id = $1 WHERE id = $2",
		newOwnerId, teamId,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func RemoveUserFromTeam(userId int64, teamId int64) error {
	_, err := Db.Exec(`DELETE FROM m_team_user WHERE user_id = $1 AND team_id = $2`, userId, teamId)
	return err
}

func RemoveTeam(teamId int64) error {
	_, err := Db.Exec(`DELETE FROM teams WHERE id = $1`, teamId)
	return err
}
