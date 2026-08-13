package init

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/security/passwordhash"
	"mosona-manager/internal/siteaccess"
	"mosona-manager/internal/utils"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

const initializationLockID int64 = 0x4d4f534f4e41494e

var (
	errAlreadyInitialized    = errors.New("system already initialized")
	errInitializationHash    = errors.New("hash initial administrator password")
	errInitializationUserAdd = errors.New("create initial administrator")
)

func initialize(c *echo.Context) error {
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
	err := initializeSystem(ctx, username, email, password, websiteUrl, registrationEnable)
	if errors.Is(err, errAlreadyInitialized) {
		return c.JSON(http.StatusConflict, _type.H{
			Code: "already_initialized",
			Msg:  "System already initialized",
		})
	}
	if errors.Is(err, errInitializationHash) {
		return c.JSON(http.StatusInternalServerError, _type.H{Code: "error", Msg: "Registration failed"})
	}
	if errors.Is(err, errInitializationUserAdd) {
		return c.JSON(http.StatusBadRequest, _type.H{Code: "error", Msg: "Registration failed"})
	}
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
	}

	// Reload dynamic configuration
	if err = db.SyncConfig(); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error: " + err.Error(),
		})
	}
	if err = siteaccess.Refresh(); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to refresh site access cache",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Initialization successful",
	})
}

func initializeSystem(ctx context.Context, username, email, password, websiteURL, registrationEnabled string) error {
	tx, err := db.Db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// This transaction-scoped lock serializes initialization even when config.init
	// does not exist yet. The database check below remains authoritative.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", initializationLockID); err != nil {
		return err
	}

	var initialized bool
	if err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM config WHERE key = 'init' AND value = 'true'
		)
	`).Scan(&initialized); err != nil {
		return err
	}
	if initialized {
		return errAlreadyInitialized
	}

	signature := utils.RandomString(32)
	hashed, err := passwordhash.Hash(password)
	if err != nil {
		return fmt.Errorf("%w: %v", errInitializationHash, err)
	}
	if _, err = tx.ExecContext(ctx,
		"INSERT INTO users (username, email, password, salt, verified, is_admin) VALUES ($1, $2, $3, $4, $5, $6)",
		username, email, hashed, signature, true, true,
	); err != nil {
		return fmt.Errorf("%w: %v", errInitializationUserAdd, err)
	}

	if err = setConfigTx(tx, ctx, "registration_enabled", registrationEnabled); err != nil {
		return err
	}
	if err = setConfigTx(tx, ctx, "domain", websiteURL); err != nil {
		return err
	}
	if err = setConfigTx(tx, ctx, "init", "true"); err != nil {
		return err
	}

	return tx.Commit()
}

func setConfigTx(tx *sql.Tx, ctx context.Context, key, value string) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO config (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2",
		key, value,
	)
	return err
}
