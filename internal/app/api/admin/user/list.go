package muser

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"strconv"

	"github.com/Masterminds/squirrel"
	"github.com/labstack/echo/v4"
)

func list(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	size, _ := strconv.Atoi(c.QueryParam("size"))

	search := c.QueryParam("search")
	verify := c.QueryParam("verify")

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	// Query
	query := psql.Select(
		"id",
		"username",
		"email",
		"is_admin",
		"verified",
		"(CASE WHEN totp IS NULL THEN false ELSE true END) AS totp_enabled",
		"created_at",
		"updated_at",
		"login_at",
	).From("users").OrderBy("id DESC").Limit(uint64(size)).Offset(uint64((page - 1) * size))

	// Count
	countQuery := psql.Select("COUNT(id)").From("users")

	// Filters
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where(squirrel.Or{
			squirrel.Eq{"id": search},
			squirrel.Like{"username": searchPattern},
			squirrel.Like{"email": searchPattern},
		})
		countQuery = countQuery.Where(squirrel.Or{
			squirrel.Eq{"id": search},
			squirrel.Like{"username": searchPattern},
			squirrel.Like{"email": searchPattern},
		})
	}
	if verify == "true" {
		query = query.Where("verified = TRUE")
		countQuery = countQuery.Where("verified = TRUE")
	} else if verify == "false" {
		query = query.Where("verified = FALSE")
		countQuery = countQuery.Where("verified = FALSE")
	}

	// To SQL
	sql, args, _ := query.ToSql()
	countSql, countArgs, _ := countQuery.ToSql()

	var users = make([]_type.User, 0)
	if err := db.Db.Select(&users, sql, args...); err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error",
		})
	}

	var total int64
	if err := db.Db.QueryRow(countSql, countArgs...).Scan(&total); err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"users": users,
			"total": total,
		},
	})
}
