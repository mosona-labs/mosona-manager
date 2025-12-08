package db

import (
	"mosona-manager/_type"
	"mosona-manager/config"
	"mosona-manager/utils"
)

func GetKeysByTeamID(tid int64) ([]_type.Key, error) {
	var keys = make([]_type.Key, 0)
	err := Db.Select(&keys,
		"SELECT id, name, updated_at, created_at FROM keys WHERE team_id = $1 ORDER BY id DESC", tid,
	)
	return keys, err
}

func AddKey(tid int64, name, content string) (int64, error) {
	k, err := utils.Encrypt([]byte(content), config.Key)
	if err != nil {
		return 0, err
	}

	var id int64
	err = Db.QueryRow(
		"INSERT INTO keys (team_id, name, content) VALUES ($1, $2, $3) RETURNING id",
		tid, name, k,
	).Scan(&id)
	return id, err
}

func UpdateKey(tid, id int64, name string) error {
	_, err := Db.Exec(
		"UPDATE keys SET name=$1, updated_at=NOW() WHERE id=$2 AND team_id=$3",
		name, id, tid,
	)
	return err
}

func DeleteKey(tid, id int64) error {
	_, err := Db.Exec("DELETE FROM keys WHERE id=$1 AND team_id=$2", id, tid)
	return err
}
