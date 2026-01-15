package db

import (
	"context"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/utils"

	"github.com/jmoiron/sqlx"
)

func GetUserById(id int64) (_type.User, error) {
	var user _type.User
	if err := Db.Get(&user, "SELECT id, username, email, is_admin, (CASE WHEN totp IS NULL THEN false ELSE true END) AS totp_enabled, verified, created_at, pwd_at FROM users WHERE id = $1", id); err != nil {
		return _type.User{}, err
	}

	return user, nil
}

func GetUserByEmail(email string) (_type.User, error) {
	var user _type.User
	if err := Db.Get(&user, "SELECT id, username, email, is_admin, created_at FROM users WHERE email = $1", email); err != nil {
		return _type.User{}, err
	}

	return user, nil
}

func GetUserAuthById(id int64) (_type.UserAuthInfo, error) {
	var user _type.UserAuthInfo
	if err := Db.Get(&user, "SELECT id, email, password, totp, salt, is_admin, verified FROM users WHERE id = $1", id); err != nil {
		return _type.UserAuthInfo{}, err
	}

	return user, nil
}

func GetUserAuthByEmail(email string) (_type.UserAuthInfo, error) {
	var user _type.UserAuthInfo
	if err := Db.Get(&user, "SELECT id, email, password, salt, is_admin, verified FROM users WHERE email = $1", email); err != nil {
		return _type.UserAuthInfo{}, err
	}

	return user, nil
}

func GetUserByIds(ids []int64) (map[int64]_type.User, error) {
	if len(ids) == 0 {
		return make(map[int64]_type.User), nil
	}

	users := make([]_type.User, 0)
	query, args, err := sqlx.In("SELECT id, username, email, is_admin, created_at FROM users WHERE id IN (?)", ids)
	if err != nil {
		return nil, err
	}
	query = Db.Rebind(query)
	if err := Db.Select(&users, query, args...); err != nil {
		return nil, err
	}

	userMap := make(map[int64]_type.User)
	for _, user := range users {
		userMap[user.ID] = user
	}

	return userMap, nil
}

func GetTeamUserIdsByEmail(ctx context.Context, teamID int64, email string) ([]int64, error) {
	var userIds []int64
	err := Db.SelectContext(ctx, &userIds, `
		SELECT u.id
		FROM users u
		JOIN m_team_user tu ON u.id = tu.user_id
		WHERE tu.team_id = $1 AND u.email LIKE $2
	`, teamID, "%"+utils.EscapeLike(email)+"%")
	if err != nil {
		return nil, err
	}

	return userIds, nil
}

func GetAdminUserIdsByEmail(ctx context.Context, email string) ([]int64, error) {
	var userIds []int64
	err := Db.SelectContext(ctx, &userIds, `
		SELECT id
		FROM users
		WHERE is_admin = true AND email LIKE $1
	`, "%"+utils.EscapeLike(email)+"%")
	if err != nil {
		return nil, err
	}

	return userIds, nil
}

func UpdateUsername(userID int64, newUsername string) error {
	_, err := Db.Exec("UPDATE users SET username = $1 WHERE id = $2", newUsername, userID)
	return err
}

func CheckEmailExists(email string) (bool, error) {
	var count int
	err := Db.Get(&count, "SELECT COUNT(1) FROM users WHERE email = $1", email)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func CheckEmailExistsExcludeID(email string, excludeID int64) (bool, error) {
	var count int
	err := Db.Get(&count, "SELECT COUNT(1) FROM users WHERE email = $1 AND id != $2", email, excludeID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func SetUserTOTP(userID int64, totp *string) error {
	_, err := Db.Exec("UPDATE users SET totp = $1 WHERE id = $2", totp, userID)
	return err
}
