package moauth

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v4"
)

func list(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	size, _ := strconv.Atoi(c.QueryParam("size"))

	var data = make([]_type.AuthProvider, 0)
	if err := db.Db.Select(
		&data,
		`SELECT id, name, icon, auth_url, token_url, userinfo_url, client_id, client_secret, skip_2fa, is_enabled, sort, created_at, updated_at FROM auth_provider ORDER BY sort, id DESC LIMIT $1 OFFSET $2`,
		size,
		(page-1)*size,
	); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	var total int64
	if err := db.Db.QueryRow(
		`SELECT COUNT(id) FROM auth_provider`,
	).Scan(&total); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"items": data,
			"total": total,
		},
	})
}
