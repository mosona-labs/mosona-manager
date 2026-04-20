package amonitor

import (
	"encoding/json"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"mosona-manager/internal/utils/store"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"
)

func chart(c *echo.Context) error {
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

	cacheKey := fmt.Sprintf("monitor:chart:%d:%s", serverId, timeFrame)
	if data, ok := store.GetChartCache(cacheKey); ok {
		return c.Blob(200, echo.MIMEApplicationJSON, data)
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
		tf = "raw_15s_avg"
	case "24h":
		startTime = endTime.Add(-24 * time.Hour)
		tf = "minute"
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

	chartData := make([]_type.ServerChartStatusType, 0, len(monitorData))
	for _, item := range monitorData {
		chartData = append(chartData, _type.ServerChartStatusType{
			CPU:           item.CPU,
			MemTotalMB:    item.MemTotalMB,
			MemUsedMB:     item.MemUsedMB,
			SwapTotalMB:   item.SwapTotalMB,
			SwapUsedMB:    item.SwapUsedMB,
			Disks:         item.Disks,
			DiskReadKibS:  item.DiskReadKibS,
			DiskWriteKibS: item.DiskWriteKibS,
			DiskReadIOPS:  item.DiskReadIOPS,
			DiskWriteIOPS: item.DiskWriteIOPS,
			RxKibS:        item.RxKibS,
			TxKibS:        item.TxKibS,
			RxTotalMB:     item.RxTotalMB,
			TxTotalMB:     item.TxTotalMB,
			Time:          item.Time,
		})
	}

	resp := _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: chartData,
	}
	body, err := json.Marshal(resp)
	if err != nil {
		return utils.ErrorHandler(c, err, "Failed to encode chart data")
	}
	store.SetChartCache(cacheKey, body, 30*time.Second)
	return c.Blob(200, echo.MIMEApplicationJSONCharsetUTF8, body)
}
