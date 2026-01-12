package admin

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v4"
)

func dashboard(c echo.Context) error {
	var (
		users, teams, servers int64
		records               int64
	)

	if err := db.Db.QueryRowContext(c.Request().Context(), `SELECT
		(SELECT COUNT(id) FROM users),
		(SELECT COUNT(id) FROM teams),
		(SELECT COUNT(id) FROM servers)
	`).Scan(&users, &teams, &servers); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	var err error
	if records, err = influx.GetAllBucketAllServerRecordCount(); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// System Usage
	usageData, err := influx.GetSystemUsage()
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"users":   users,
			"teams":   teams,
			"servers": servers,
			"records": records,
			"system":  usageData,
		},
	})
}
