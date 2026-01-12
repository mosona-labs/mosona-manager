package amonitor

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

func chart(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	serverId, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	timeFrame := c.QueryParam("time_frame")

	if tid == 0 || serverId == 0 || timeFrame == "" {
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

	var tf string
	var startTime time.Time
	endTime := time.Now()
	switch timeFrame {
	case "1h":
		startTime = endTime.Add(-1 * time.Hour)
		tf = "raw"
	case "12h":
		startTime = endTime.Add(-12 * time.Hour)
		tf = "raw"
	case "24h":
		startTime = endTime.Add(-24 * time.Hour)
		tf = "raw"
	case "7d":
		startTime = endTime.Add(-7 * 24 * time.Hour)
		tf = "minute"
	case "30d":
		startTime = endTime.Add(-30 * 24 * time.Hour)
		tf = "hour"
	case "180d":
		startTime = endTime.Add(-180 * 24 * time.Hour)
		tf = "day"
	case "365d":
		startTime = endTime.Add(-365 * 24 * time.Hour)
		tf = "day"
	default:
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid time frame",
		})
	}

	monitorData, err := influx.GetServerStatusHistory(serverId, startTime, endTime, tf)
	if err != nil {
		return utils.ErrorHandler(c, err, "InfluxDB error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: monitorData,
	})
}
