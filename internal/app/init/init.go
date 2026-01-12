package init

import (
	"context"
	"database/sql"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"strings"

	"github.com/labstack/echo/v4"
)

func initialize(c echo.Context) error {
	username := strings.TrimSpace(c.FormValue("username"))
	email := strings.TrimSpace(c.FormValue("email"))
	password := c.FormValue("password")

	websiteUrl := strings.TrimSpace(c.FormValue("website_url"))
	registrationEnable := c.FormValue("registration_enable")

	if username == "" || email == "" || password == "" || websiteUrl == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Fields cannot be empty",
		})
	}
	if !strings.Contains(email, "@") {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Email format is incorrect",
		})
	}
	if !strings.HasPrefix(websiteUrl, "http://") && !strings.HasPrefix(websiteUrl, "https://") {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Website URL format is incorrect",
		})
	}

	ctx := c.Request().Context()

	tx, err := db.Db.Begin()
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
	}

	signature := utils.RandomString(32)
	if _, err = tx.ExecContext(ctx,
		"INSERT INTO users (username, email, password, salt, verified, is_admin) VALUES ($1, $2, $3, $4, $5, $6)",
		username, email, utils.SHA256(password+signature+config.DynamicConf.Token), signature, true, true,
	); err != nil {
		_ = tx.Rollback()
		return c.JSON(400, _type.H{Code: "error", Msg: "Registration failed"})
	}

	if err = setConfigTx(tx, ctx, "registration_enabled", registrationEnable); err != nil {
		_ = tx.Rollback()
		return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
	}
	if err = setConfigTx(tx, ctx, "website_url", websiteUrl); err != nil {
		_ = tx.Rollback()
		return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
	}
	if err = setConfigTx(tx, ctx, "init", "true"); err != nil {
		_ = tx.Rollback()
		return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
	}

	if err = tx.Commit(); err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
	}

	// Reload dynamic configuration
	if err = db.SyncConfig(); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error: " + err.Error(),
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Initialization successful",
	})
}

func setConfigTx(tx *sql.Tx, ctx context.Context, key, value string) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO config (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2",
		key, value,
	)
	return err
}
