package amonitor

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/pkg/_type"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

func get(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)

	serverId, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if tid == 0 || serverId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid server monitor data",
		})
	}

	data, err := db.GetMonitoredServerInfo(tid, serverId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(500, _type.H{
				Code: "empty",
				Msg:  "No monitored server found",
			})
		} else {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Database error",
			})
		}
	}

	latestStatus, err := influx.GetLatestServerStatus(serverId)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to get server status",
		})
	}
	stale := latestStatus.Time.IsZero() || time.Since(latestStatus.Time) > 5*time.Second

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"info":  data,
			"now":   time.Now(),
			"stale": stale,
		},
	})
}
