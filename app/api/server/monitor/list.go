package amonitor

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"mosona-manager/_type"
	"mosona-manager/db"
	"mosona-manager/influx"
	"time"
)

func list(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)

	servers, err := db.ListMonitoredServers(tid)
	if err != nil {
		fmt.Println(err)
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to list monitored servers",
		})
	}

	var ids []int64
	for _, server := range servers {
		ids = append(ids, server.ID)
	}
	statusMap, err := influx.GetLatestServerStatusBatch(ids)
	if err != nil {
		fmt.Println(err)
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to get server statuses",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"servers": servers,
			"status":  statusMap,
			"now":     time.Now().Unix(),
		},
	})
}
