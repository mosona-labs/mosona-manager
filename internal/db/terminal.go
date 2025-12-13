package db

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/utils/encrypt"

	"github.com/Masterminds/squirrel"
)

func ListTerminals(teamId int64) ([]_type.Terminal, error) {
	var servers = make([]_type.Terminal, 0)

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	query := psql.Select(
		"s.id", "s.type", "s.name", "s.weight", "s.category",
		"i.os",
		"COALESCE(ssh.username, NULL)",
		"COALESCE(ssh.address, NULL)",
		"COALESCE(ssh.port, NULL)",
	).
		From("servers s").
		LeftJoin("server_info i ON s.id = i.sid").
		LeftJoin("ssh ON s.id = ssh.server_id").
		Where(squirrel.Eq{
			"team_id":        teamId,
			"allow_terminal": true,
		}).
		OrderBy("weight DESC, id DESC")

	sql, args, err := query.ToSql()
	if err != nil {
		return servers, err
	}

	rows, err := Db.Query(sql, args...)
	if err != nil {
		return servers, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var server _type.Terminal
		if err = rows.Scan(
			&server.ID, &server.Type, &server.Name, &server.Weight, &server.Category,
			&server.OS,
			&server.Username, &server.Address, &server.Port,
		); err != nil {
			return servers, err
		}
		servers = append(servers, server)
	}

	return servers, err
}

func GetTerminalInfo(teamId, serverId int64) (_type.TerminalDetail, error) {
	var server _type.TerminalDetail
	var password []byte

	if err := Db.QueryRow(
		"SELECT address, port, username, password FROM servers s JOIN ssh on s.id = ssh.server_id WHERE s.id = $1 AND team_id = $2 AND allow_terminal = true",
		serverId, teamId,
	).Scan(
		&server.Address, &server.Port, &server.Username, &password,
	); err != nil {
		return server, err
	}

	pwd, err := encrypt.Decrypt(password, encrypt.Key)
	if err != nil {
		return server, err
	}
	server.Password = string(pwd)

	return server, nil
}
