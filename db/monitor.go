package db

import (
	"github.com/Masterminds/squirrel"
	"mosona-manager/_type"
)

func ListMonitoredServers(teamId int64) ([]_type.Monitor, error) {
	var servers = make([]_type.Monitor, 0)

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	query := psql.Select(
		"s.id as id", "name", "weight", "category",
		"i.os", "i.county", "i.area", "i.open_time", "i.provider", "i.cycle",
		"i.start_time", "i.end_time", "i.amount", "i.bandwidth", "i.traffic", "i.traffic_type",
		"i.note_public",
	).
		From("servers s").
		LeftJoin("server_info i ON s.id = i.sid").
		Where(squirrel.Eq{
			"s.team_id":       teamId,
			"s.allow_monitor": true,
		}).
		OrderBy("weight DESC, s.id DESC")

	sql, args, err := query.ToSql()
	if err != nil {
		return servers, err
	}
	err = Db.Select(&servers, sql, args...)
	return servers, err
}
