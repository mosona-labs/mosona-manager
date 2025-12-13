package db

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/utils/encrypt"

	"github.com/Masterminds/squirrel"
)

func GetKeysByTeamID(tid int64) ([]_type.Key, error) {
	var keys = make([]_type.Key, 0)
	err := Db.Select(&keys,
		"SELECT id, name, updated_at, created_at FROM keys WHERE team_id = $1 ORDER BY id DESC", tid,
	)
	return keys, err
}

func AddKey(tid int64, name, content, password string) (int64, error) {
	k, err := encrypt.Encrypt([]byte(content), encrypt.Key)
	if err != nil {
		return 0, err
	}
	var pwd []byte
	if password != "" {
		pwd, err = encrypt.Encrypt([]byte(password), encrypt.Key)
		if err != nil {
			return 0, err
		}
	}

	var id int64
	err = Db.QueryRow(
		"INSERT INTO keys (team_id, name, content, password) VALUES ($1, $2, $3, $4) RETURNING id",
		tid, name, k, pwd,
	).Scan(&id)
	return id, err
}

func UpdateKey(tid, id int64, name, password string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	query := psql.Update("keys").Set("name", name).Where(squirrel.Eq{"id": id, "team_id": tid})

	if password == "!msn!empty!" {
		query = query.Set("password", nil)
	} else if password != "" {
		pwd, err := encrypt.Encrypt([]byte(password), encrypt.Key)
		if err != nil {
			return err
		}
		query = query.Set("password", pwd)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = Db.Exec(sql, args...)
	return err
}

func DeleteKey(tid, id int64) error {
	_, err := Db.Exec("DELETE FROM keys WHERE id=$1 AND team_id=$2", id, tid)
	return err
}
