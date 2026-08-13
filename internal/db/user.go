package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/utils"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

var (
	ErrDeleteUserConfirmationMismatch = errors.New("delete user confirmation does not match")
	ErrCannotModifySelf               = errors.New("administrator cannot delete or demote self")
	ErrLastAdmin                      = errors.New("operation would remove the last administrator")
	ErrAdminReauthenticationRequired  = errors.New("administrator reauthentication required")
	ErrActorNotAdmin                  = errors.New("acting user is no longer an administrator")
	ErrUserEmailExists                = errors.New("user email already exists")
)

type AdminUserUpdate struct {
	Username     string
	Email        string
	Verified     bool
	IsAdmin      bool
	PasswordHash string
	PasswordSalt string
}

type OwnedTeam struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type UserOwnsTeamsError struct {
	Teams []OwnedTeam
}

func (err *UserOwnsTeamsError) Error() string {
	return fmt.Sprintf("user owns %d team(s)", len(err.Teams))
}

func DeleteUser(ctx context.Context, actorID, userID int64, confirmation, reauthenticatedHash string) (string, error) {
	if actorID == userID {
		return "", ErrCannotModifySelf
	}
	if reauthenticatedHash == "" {
		return "", ErrAdminReauthenticationRequired
	}

	tx, err := Db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	admins, err := lockAdministrators(ctx, tx)
	if err != nil {
		return "", err
	}
	actorHash, actorIsAdmin := administratorPassword(admins, actorID)
	if !actorIsAdmin {
		return "", ErrActorNotAdmin
	}
	if actorHash != reauthenticatedHash {
		return "", ErrAdminReauthenticationRequired
	}

	var username string
	var isAdmin bool
	if err = tx.QueryRowContext(ctx,
		"SELECT username, is_admin FROM users WHERE id = $1 FOR UPDATE",
		userID,
	).Scan(&username, &isAdmin); err != nil {
		return "", err
	}
	if confirmation == "" || confirmation != username {
		return "", ErrDeleteUserConfirmationMismatch
	}
	if isAdmin && len(admins) == 1 {
		return "", ErrLastAdmin
	}

	ownedTeams := make([]OwnedTeam, 0)
	if err = tx.SelectContext(ctx, &ownedTeams,
		"SELECT id, name FROM teams WHERE owner_id = $1 ORDER BY id",
		userID,
	); err != nil {
		return "", err
	}
	if len(ownedTeams) != 0 {
		return "", &UserOwnsTeamsError{Teams: ownedTeams}
	}

	result, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
	if err != nil {
		return "", err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if affected != 1 {
		return "", sql.ErrNoRows
	}

	if err = tx.Commit(); err != nil {
		return "", err
	}
	return username, nil
}

func UpdateAdminUser(ctx context.Context, actorID, userID int64, update AdminUserUpdate, reauthenticatedHash string) error {
	if actorID == userID && !update.IsAdmin {
		return ErrCannotModifySelf
	}

	tx, err := Db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	admins, err := lockAdministrators(ctx, tx)
	if err != nil {
		return err
	}
	actorHash, actorIsAdmin := administratorPassword(admins, actorID)
	if !actorIsAdmin {
		return ErrActorNotAdmin
	}

	var targetIsAdmin bool
	if err = tx.QueryRowContext(ctx,
		"SELECT is_admin FROM users WHERE id = $1 FOR UPDATE",
		userID,
	).Scan(&targetIsAdmin); err != nil {
		return err
	}
	if targetIsAdmin && !update.IsAdmin {
		if actorID == userID {
			return ErrCannotModifySelf
		}
		if len(admins) == 1 {
			return ErrLastAdmin
		}
	}
	if targetIsAdmin != update.IsAdmin || update.PasswordHash != "" {
		if reauthenticatedHash == "" || actorHash != reauthenticatedHash {
			return ErrAdminReauthenticationRequired
		}
	}

	var result sql.Result
	if update.PasswordHash == "" {
		result, err = tx.ExecContext(ctx, `
			UPDATE users
			SET username = $1, email = $2, verified = $3, is_admin = $4
			WHERE id = $5
		`, update.Username, update.Email, update.Verified, update.IsAdmin, userID)
	} else {
		result, err = tx.ExecContext(ctx, `
			UPDATE users
			SET username = $1, email = $2, verified = $3, is_admin = $4,
				password = $5, salt = $6, pwd_at = now()
			WHERE id = $7
		`, update.Username, update.Email, update.Verified, update.IsAdmin,
			update.PasswordHash, update.PasswordSalt, userID)
	}
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" && pqErr.Constraint == "users_email_unique" {
			return ErrUserEmailExists
		}
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

type administratorCredential struct {
	ID       int64
	Password string
}

func lockAdministrators(ctx context.Context, tx *sqlx.Tx) ([]administratorCredential, error) {
	admins := make([]administratorCredential, 0)
	if err := tx.SelectContext(ctx, &admins,
		"SELECT id, password FROM users WHERE is_admin = true ORDER BY id FOR UPDATE",
	); err != nil {
		return nil, err
	}
	return admins, nil
}

func administratorPassword(admins []administratorCredential, userID int64) (string, bool) {
	for _, admin := range admins {
		if admin.ID == userID {
			return admin.Password, true
		}
	}
	return "", false
}

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
	if err := Db.Get(&user, "SELECT id, email, password, salt, totp, is_admin, verified FROM users WHERE email = $1", email); err != nil {
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

func SetUserTOTP(userID int64, totp *string) error {
	_, err := Db.Exec("UPDATE users SET totp = $1 WHERE id = $2", totp, userID)
	return err
}
