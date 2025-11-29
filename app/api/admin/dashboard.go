package admin

import (
	"mosona-manager/_type"
	"mosona-manager/db"
	"mosona-manager/influx"

	"github.com/labstack/echo/v4"
)

func dashboard(c echo.Context) error {
	var (
		users   int64
		teams   int64
		servers int64
		records int64
	)

	if err := db.Db.QueryRow("SELECT COUNT(id) FROM users").Scan(&users); err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error",
		})
	}
	if err := db.Db.QueryRow("SELECT COUNT(id) FROM teams").Scan(&teams); err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error",
		})
	}
	if err := db.Db.QueryRow("SELECT COUNT(id) FROM servers").Scan(&servers); err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error",
		})
	}
	var err error
	if records, err = influx.GetAllBucketAllServerRecordCount(); err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error",
		})
	}

	// System Usage
	usageData, err := influx.GetSystemUsage()
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error",
		})
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
