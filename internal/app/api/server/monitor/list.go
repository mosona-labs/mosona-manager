package amonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/access"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

func list(c *echo.Context) error {
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
		Data: _type.Map{
			"servers": servers,
			"status":  statusMap,
			"now":     time.Now().Unix(),
		},
	})
}

func sse(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	tid, _ := c.Get("tid").(int64)
	sid, _ := c.Get("sid").(string)

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(200)

	ctx, cancel := context.WithCancel(c.Request().Context())
	defer cancel()

	sendData := func() bool {
		if err := access.ValidateTeamSession(ctx, uid, tid, sid, 0, 1, 2); err != nil {
			event := "error"
			code := "error"
			message := "Authorization check failed"
			if errors.Is(err, access.ErrTeamAccessRevoked) {
				event = "revoked"
				code = "team_access_revoked"
				message = "Team access has been revoked"
			}
			data, _ := json.Marshal(_type.Map{"code": code, "msg": message})
			_, _ = fmt.Fprintf(c.Response(), "event: %s\ndata: %s\n\n", event, data)
			if flusher, ok := c.Response().(http.Flusher); ok {
				flusher.Flush()
			}
			return false
		}
		servers, err := db.ListMonitoredServers(tid)
		if err != nil {
			_, _ = fmt.Fprintf(c.Response(), "event: error\ndata: {\"msg\":\"Failed to list monitored servers\"}\n\n")
			if flusher, ok := c.Response().(http.Flusher); ok {
				flusher.Flush()
			}
			return true
		}

		var ids []int64
		for _, server := range servers {
			ids = append(ids, server.ID)
		}

		statusMap, err := influx.GetLatestServerStatusBatch(ids)
		if err != nil {
			_, _ = fmt.Fprintf(c.Response(), "event: error\ndata: {\"msg\":\"Failed to get server statuses\"}\n\n")
			if flusher, ok := c.Response().(http.Flusher); ok {
				flusher.Flush()
			}
			return true
		}

		data, _ := json.Marshal(_type.Map{
			"servers": servers,
			"status":  statusMap,
			"now":     time.Now().Unix(),
		})

		_, _ = fmt.Fprintf(c.Response(), "event: update\ndata: %s\n\n", string(data))
		if flusher, ok := c.Response().(http.Flusher); ok {
			flusher.Flush()
		}
		return true
	}

	if !sendData() {
		return nil
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if !sendData() {
				return nil
			}
		}
	}
}
