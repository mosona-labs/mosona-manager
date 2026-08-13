package msettings

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/oauth"
	"mosona-manager/internal/siteaccess"
	"mosona-manager/internal/utils"
	"strings"

	"github.com/labstack/echo/v5"
)

type setRequest struct {
	Key   string `json:"key" validate:"required"`
	Value string `json:"value" validate:"required"`
}

var addSettingsAuditLog = influx.LogAdd

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
	defer func() { _ = tx.Rollback() }()

	stmt := `INSERT INTO config (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2`
	applied := make([]setRequest, 0, len(*req))
	for _, item := range *req {
		// Skip protected keys
		if item.Key == "init" || item.Key == "token" || item.Key == "favicon" {
			continue
		}
		// The API returns this mask for configured secrets. Treating it as a
		// value would overwrite the existing credential during an unrelated save.
		if isSensitiveSetting(item.Key) && item.Value == secretMask {
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
		applied = append(applied, item)
	}

	if err = tx.Commit(); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Reload dynamic configuration
	if err = db.SyncConfig(); err != nil {
		return utils.ErrorHandler(c, err, "Failed to reload configuration")
	}

	domainChanged := false
	for _, item := range applied {
		if item.Key == "domain" {
			domainChanged = true
			break
		}
	}
	if domainChanged {
		if err := siteaccess.Refresh(); err != nil {
			return utils.ErrorHandler(c, err, "Failed to refresh site access cache")
		}
		oauth.RefreshRedirectURLs()
	}

	// Log action
	settings := make([]string, 0, len(applied))
	for _, item := range applied {
		if isSensitiveSetting(item.Key) {
			settings = append(settings, item.Key+" updated")
			continue
		}
		settings = append(settings, item.Key+"="+item.Value)
	}
	if len(settings) > 0 {
		addSettingsAuditLog(
			0, c.Get("uid").(int64), "settings", "updated: "+strings.Join(settings, ", "),
			c.RealIP(), c.Request().UserAgent(), "medium",
		)
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Settings updated successfully",
	})
}
