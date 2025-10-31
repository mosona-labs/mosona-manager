package amonitor

import (
	"mosona-manager/_type"
	"mosona-manager/db"
	"mosona-manager/influx"
	"strconv"

	"github.com/labstack/echo/v4"
)

func realTime(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	serverId, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	if tid == 0 || serverId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid request data",
		})
	}

	var exist int
	if err := db.Db.Get(
		&exist,
		"SELECT COUNT(1) FROM servers WHERE team_id=$1 AND id=$2",
		tid, serverId,
	); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	if exist == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Server not found",
		})
	}

	latestStatus, err := influx.GetLatestServerStatus(serverId)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to get server status",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: latestStatus,
	})
}
