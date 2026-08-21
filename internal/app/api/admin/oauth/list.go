package moauth

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/app/api/pagination"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v5"
)

func list(c *echo.Context) error {
	page, size, err := pagination.ParseOffset(c.QueryParam("page"), c.QueryParam("size"))
	if err != nil {
		return c.JSON(400, _type.H{Code: "invalid", Msg: "Invalid pagination"})
	}

	var data = make([]_type.AuthProvider, 0)
	if err := db.Db.Select(
		&data,
		`SELECT id, name, icon, protocol, issuer_url, auth_url, token_url, userinfo_url, scopes, subject_field, identity_namespace_version, config_revision, client_id, client_secret, skip_2fa, is_enabled, sort, created_at, updated_at FROM auth_provider ORDER BY sort, id DESC LIMIT $1 OFFSET $2`,
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
		Data: _type.Map{
			"items": data,
			"total": total,
		},
	})
}
