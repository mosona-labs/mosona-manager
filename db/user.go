package db

import (
	"mosona-manager/_type"

	"github.com/jmoiron/sqlx"
)

func GetUserById(id int64) (_type.User, error) {
	var user _type.User
	if err := Db.Get(&user, "SELECT id, username, email, is_admin, created_at FROM users WHERE id = $1", id); err != nil {
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

func GetUserAuthByEmail(email string) (_type.UserAuthInfo, error) {
	var user _type.UserAuthInfo

	if err := Db.Get(&user, "SELECT id, email, password, salt, is_admin, verified FROM users WHERE email = $1", email); err != nil {
		return _type.UserAuthInfo{}, err
	}

	return user, nil
}

func GetUserByIds(ids []int64) (map[int64]_type.User, error) {
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
