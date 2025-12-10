package db

import (
	"mosona-manager/internal/config"
	"mosona-manager/internal/utils"
	"mosona-manager/pkg/_type"

	"github.com/Masterminds/squirrel"
)

func ListTerminals(teamId int64) ([]_type.Terminal, error) {
	var servers = make([]_type.Terminal, 0)

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	query := psql.Select(
		"s.id", "s.name", "s.weight", "s.category",
		"s.username", "s.address", "s.port",
		"i.os",
	).
		From("servers s").
		LeftJoin("server_info i ON s.id = i.sid").
		Where(squirrel.Eq{
			"team_id":        teamId,
			"allow_terminal": true,
		}).
		OrderBy("weight DESC, id DESC")

	sql, args, err := query.ToSql()
	if err != nil {
		return servers, err
	}
	err = Db.Select(&servers, sql, args...)
	return servers, err
}

func GetTerminalInfo(teamId, serverId int64) (_type.TerminalDetail, error) {
	var server _type.TerminalDetail
	var password []byte

	if err := Db.QueryRow(
		"SELECT address, port, username, password FROM servers WHERE id = $1 AND team_id = $2 AND allow_terminal = true",
		serverId, teamId,
	).Scan(
		&server.Address, &server.Port, &server.Username, &password,
	); err != nil {
		return server, err
	}

	pwd, err := utils.Decrypt(password, config.Key)
	if err != nil {
		return server, err
	}
	server.Password = string(pwd)

	return server, nil
}
