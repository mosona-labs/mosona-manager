package msettings

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/oauth"
	"mosona-manager/internal/utils"
	"strings"

	"github.com/labstack/echo/v5"
)

type setRequest struct {
	Key   string `json:"key" validate:"required"`
	Value string `json:"value" validate:"required"`
}

func set(c *echo.Context) error {
	req := new([]setRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid request format",
		})
	}

	tx, err := db.Db.Begin()
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	stmt := `INSERT INTO config (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2`
	for _, item := range *req {
		// Skip protected keys
		if item.Key == "init" || item.Key == "token" || item.Key == "favicon" {
			continue
		}
		if item.Key == "title" {
			item.Value = strings.TrimSpace(item.Value)
			if len(item.Value) > 255 {
				_ = tx.Rollback()
				return c.JSON(400, _type.H{
					Code: "invalid",
					Msg:  "Title is too long",
				})
			}
		}
		// Execute upsert
		if _, err = tx.Exec(stmt, item.Key, item.Value); err != nil {
			_ = tx.Rollback()
			return utils.ErrorHandler(c, err, "Database error")
		}
	}

	if err = tx.Commit(); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Reload dynamic configuration
	if err = db.SyncConfig(); err != nil {
		return utils.ErrorHandler(c, err, "Failed to reload configuration")
	}

	// Reinitialize OAuth if domain changed
	for _, item := range *req {
		switch item.Key {
		case "domain":
			oauth.Init()
		}
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
