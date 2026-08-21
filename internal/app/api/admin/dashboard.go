package admin

import (
	"context"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"time"

	"github.com/labstack/echo/v5"
)

const dashboardQueryTimeout = 15 * time.Second

func dashboard(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), dashboardQueryTimeout)
	defer cancel()

	var (
		users, teams, servers int64
		records               int64
	)

	if err := db.Db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(id) FROM users),
		(SELECT COUNT(id) FROM teams),
		(SELECT COUNT(id) FROM servers)
	`).Scan(&users, &teams, &servers); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	type recordCountResult struct {
		count int64
		err   error
	}
	type systemUsageResult struct {
		data []*_type.ServerUsageRecord
		err  error
	}
	recordsResultCh := make(chan recordCountResult, 1)
	usageResultCh := make(chan systemUsageResult, 1)
	go func() {
		count, err := influx.GetAllBucketAllServerRecordCountContext(ctx)
		recordsResultCh <- recordCountResult{count: count, err: err}
	}()
	go func() {
		data, err := influx.GetSystemUsageContext(ctx)
		usageResultCh <- systemUsageResult{data: data, err: err}
	}()

	recordsResult := <-recordsResultCh
	if recordsResult.err != nil {
		return utils.ErrorHandler(c, recordsResult.err, "Database error")
	}
	records = recordsResult.count

	usageResult := <-usageResultCh
	if usageResult.err != nil {
		return utils.ErrorHandler(c, usageResult.err, "Database error")
	}
	usageData := usageResult.data

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: _type.Map{
			"users":   users,
			"teams":   teams,
			"servers": servers,
			"records": records,
			"system":  usageData,
		},
	})
}
