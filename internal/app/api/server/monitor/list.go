package amonitor

import (
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

const (
	monitorSessionCheckInterval = 30 * time.Second
	monitorSSEWriteTimeout      = 5 * time.Second
)

var monitorSSEErrorEvent = []byte("event: error\ndata: {\"msg\":\"Failed to load monitor data\"}\n\n")

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

	ctx := c.Request().Context()
	authErr := access.ValidateTeamSession(ctx, uid, tid, sid, 0, 1, 2)
	var initial monitorSnapshotResult
	var updates <-chan monitorSnapshotResult
	var unsubscribe func()
	if authErr == nil {
		var source *monitorSnapshotSource
		source, updates, unsubscribe = monitorSnapshots.subscribe(tid)
		defer unsubscribe()
		initial = source.get(ctx)
		if ctx.Err() != nil {
			return nil
		}
	}

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache, no-store")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	write := func(event []byte) error {
		controller := http.NewResponseController(c.Response())
		if err := controller.SetWriteDeadline(time.Now().Add(monitorSSEWriteTimeout)); err == nil {
			defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
		}
		if _, err := c.Response().Write(event); err != nil {
			return err
		}
		return controller.Flush()
	}
	writeAuthError := func(err error) error {
		event := "error"
		code := "error"
		message := "Authorization check failed"
		if errors.Is(err, access.ErrTeamAccessRevoked) {
			event = "revoked"
			code = "team_access_revoked"
			message = "Team access has been revoked"
		}
		data, _ := json.Marshal(_type.Map{"code": code, "msg": message})
		return write(fmt.Appendf(nil, "event: %s\ndata: %s\n\n", event, data))
	}
	writeResult := func(result monitorSnapshotResult) error {
		if result.err != nil {
			return write(monitorSSEErrorEvent)
		}
		return write(fmt.Appendf(nil, "event: update\ndata: %s\n\n", result.data))
	}

	if authErr != nil {
		_ = writeAuthError(authErr)
		return nil
	}
	if err := writeResult(initial); err != nil {
		return nil
	}

	authTicker := time.NewTicker(monitorSessionCheckInterval)
	defer authTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-authTicker.C:
			if err := access.ValidateTeamSession(ctx, uid, tid, sid, 0, 1, 2); err != nil {
				_ = writeAuthError(err)
				return nil
			}
		case result := <-updates:
			if err := writeResult(result); err != nil {
				return nil
			}
		}
	}
}
