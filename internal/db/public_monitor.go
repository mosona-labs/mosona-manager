package db

import (
	"mosona-manager/internal/_type"

	"github.com/Masterminds/squirrel"
)

func ListPublicMonitoredServers(teamID int64) ([]_type.PublicMonitor, error) {
	servers := make([]_type.PublicMonitor, 0)

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	query := psql.Select(
		"s.id AS id", "s.name", "s.category", "s.weight",
		"i.os", "i.county", "i.area", "i.open_time", "i.provider", "i.cycle",
		"i.start_time", "i.end_time", "i.amount", "i.bandwidth", "i.traffic", "i.traffic_type",
		"i.note_public",
	).
		From("servers s").
		LeftJoin("server_info i ON s.id = i.sid").
		Where(squirrel.Eq{
			"s.team_id":       teamID,
			"s.allow_monitor": true,
		}).
		OrderBy("s.weight DESC, s.id DESC")

	sql, args, err := query.ToSql()
	if err != nil {
		return servers, err
	}

	err = Db.Select(&servers, sql, args...)
	return servers, err
}
