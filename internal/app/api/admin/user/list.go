package muser

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/app/api/pagination"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/Masterminds/squirrel"
	"github.com/labstack/echo/v5"
)

func list(c *echo.Context) error {
	page, size, err := pagination.ParseOffset(c.QueryParam("page"), c.QueryParam("size"))
	if err != nil {
		return c.JSON(400, _type.H{Code: "invalid", Msg: "Invalid pagination"})
	}

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
		searchFilter := userSearchFilter(search)
		query = query.Where(searchFilter)
		countQuery = countQuery.Where(searchFilter)
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
		return utils.ErrorHandler(c, err, "Database error")
	}

	var total int64
	if err := db.Db.QueryRow(countSql, countArgs...).Scan(&total); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: _type.Map{
			"users": users,
			"total": total,
		},
	})
}

func userSearchFilter(search string) squirrel.Sqlizer {
	searchPattern := "%" + utils.EscapeLike(search) + "%"
	conditions := squirrel.Or{}
	if userID, err := strconv.ParseInt(search, 10, 64); err == nil {
		conditions = append(conditions, squirrel.Eq{"id": userID})
	}
	return append(conditions,
		squirrel.Expr(`username LIKE ? ESCAPE '\'`, searchPattern),
		squirrel.Expr(`email LIKE ? ESCAPE '\'`, searchPattern),
	)
}
