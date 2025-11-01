package db

import (
	"mosona-manager/_type"

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
