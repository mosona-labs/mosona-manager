package amonitor

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v5"
)

func realTime(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	serverId, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	if tid == 0 || serverId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid request data",
		})
	}

	exist, err := db.IsServerExists(tid, serverId)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	if !exist {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Server not found",
		})
	}

	latestStatus, err := influx.GetLatestServerStatus(serverId)
	if err != nil {
		return utils.ErrorHandler(c, err, "Failed to get server status")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: latestStatus,
	})
}
