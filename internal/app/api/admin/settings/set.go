package msettings

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/oauth"
	"strings"

	"github.com/labstack/echo/v4"
)

type setRequest struct {
	Key   string `json:"key" validate:"required"`
	Value string `json:"value" validate:"required"`
}

func set(c echo.Context) error {
	req := new([]setRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid request format",
		})
	}

	tx, err := db.Db.Begin()
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error",
		})
	}

	stmt := `INSERT INTO config (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2`
	for _, item := range *req {
		// Skip protected keys
		if item.Key == "init" || item.Key == "token" {
			continue
		} else if item.Key == "domain" {
			oauth.Init()
		}
		// Execute upsert
		if _, err = tx.Exec(stmt, item.Key, item.Value); err != nil {
			_ = tx.Rollback()
			return c.JSON(500, _type.H{
				Code: "err",
				Msg:  "Database error",
			})
		}
	}

	if err = tx.Commit(); err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error",
		})
	}

	// Reload dynamic configuration
	if err = db.SyncConfig(); err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Failed to reload configuration",
		})
	}

	// Log action
	var settings []string
	for _, item := range *req {
		settings = append(settings, item.Key+"="+item.Value)
	}
	influx.LogAdd(
		0, c.Get("uid").(int64), "settings", "updated: "+strings.Join(settings, ", "),
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Settings updated successfully",
	})
}
