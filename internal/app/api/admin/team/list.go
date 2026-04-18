package mteam

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/Masterminds/squirrel"
	"github.com/labstack/echo/v5"
)

func list(c *echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	size, _ := strconv.Atoi(c.QueryParam("size"))

	search := c.QueryParam("search")
	email := c.QueryParam("email")

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	// Query
	query := psql.Select(
		"id",
		"name",
		"description",
		"created_at",
		"updated_at",
	).From("teams").
		Join("m_team_user mtu ON teams.id = mtu.team_id").
		Join("users u ON mtu.user_id = u.id").
		OrderBy("id DESC").
		Limit(uint64(size)).Offset(uint64((page - 1) * size))

	// Count
	countQuery := psql.Select("COUNT(DISTINCT teams.id)").From("teams").
		Join("m_team_user mtu ON teams.id = mtu.team_id").
		Join("users u ON mtu.user_id = u.id")

	// Filters
	if email != "" {
		query = query.Where(squirrel.Like{"u.email": "%" + email + "%"})
		countQuery = countQuery.Where(squirrel.Like{"u.email": "%" + email + "%"})
	}
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where(squirrel.Or{
			squirrel.Eq{"teams.id": search},
			squirrel.Like{"teams.name": searchPattern},
			squirrel.Like{"teams.description": searchPattern},
		})
		countQuery = countQuery.Where(squirrel.Or{
			squirrel.Eq{"teams.id": search},
			squirrel.Like{"teams.name": searchPattern},
			squirrel.Like{"teams.description": searchPattern},
		})
	}

	// To SQL
	sql, args, _ := query.ToSql()
	countSql, countArgs, _ := countQuery.ToSql()

	var teams = make([]_type.Team, 0)
	if err := db.Db.Select(&teams, sql, args...); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	var total int64
	if err := db.Db.Get(&total, countSql, countArgs...); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: _type.Map{
			"teams": teams,
			"total": total,
		},
	})
}
