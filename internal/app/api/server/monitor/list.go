package amonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"time"

	"github.com/labstack/echo/v4"
)

func list(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)

	servers, err := db.ListMonitoredServers(tid)
	if err != nil {
		return utils.ErrorHandler(c, err, "Failed to list monitored servers")
	}

	var ids []int64
	for _, server := range servers {
		ids = append(ids, server.ID)
	}
	statusMap, err := influx.GetLatestServerStatusBatch(ids)
	if err != nil {
		return utils.ErrorHandler(c, err, "Failed to get server statuses")
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

func sse(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(200)

	ctx, cancel := context.WithCancel(c.Request().Context())
	defer cancel()

	sendData := func() {
		servers, err := db.ListMonitoredServers(tid)
		if err != nil {
			_, _ = fmt.Fprintf(c.Response(), "event: error\ndata: {\"msg\":\"Failed to list monitored servers\"}\n\n")
			c.Response().Flush()
			return
		}

		var ids []int64
		for _, server := range servers {
			ids = append(ids, server.ID)
		}

		statusMap, err := influx.GetLatestServerStatusBatch(ids)
		if err != nil {
			_, _ = fmt.Fprintf(c.Response(), "event: error\ndata: {\"msg\":\"Failed to get server statuses\"}\n\n")
			c.Response().Flush()
			return
		}

		data, _ := json.Marshal(echo.Map{
			"servers": servers,
			"status":  statusMap,
			"now":     time.Now().Unix(),
		})

		_, _ = fmt.Fprintf(c.Response(), "event: update\ndata: %s\n\n", string(data))
		c.Response().Flush()
	}

	sendData()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			sendData()
		}
	}
}
