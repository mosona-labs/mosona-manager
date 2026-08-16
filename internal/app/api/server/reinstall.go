package aserver

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/connect/conn"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"mosona-manager/pkg/identity"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

var (
	reinstallLogAdd          = influx.LogAdd
	reinstallStopServer      = conn.StopServer
	reinstallReconcileServer = conn.ReconcileServer
)

func reinstall(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	tid, _ := c.Get("tid").(int64)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid server ID",
		})
	}
	mode, err := strconv.Atoi(c.FormValue("mode"))
	if err != nil || (mode != 1 && mode != 2) {
		return c.JSON(400, _type.H{
			Code: "invalid_mode",
			Msg:  "Reinstall mode must be active or passive Agent",
		})
	}
	address := strings.TrimSpace(c.FormValue("address"))
	port, portErr := strconv.Atoi(c.FormValue("port"))
	if mode == 1 && (address == "" || portErr != nil || port < 1 || port > 65535) {
		return c.JSON(400, _type.H{
			Code: "invalid_agent_address",
			Msg:  "Active Agent address and port are required",
		})
	}

	ctx := c.Request().Context()
	tx, err := db.Db.BeginTx(ctx, nil)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	defer func() { _ = tx.Rollback() }()
	var currentType int16
	if err = tx.QueryRowContext(ctx,
		"SELECT type FROM servers WHERE id = $1 AND team_id = $2 FOR UPDATE",
		id, tid,
	).Scan(&currentType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(400, _type.H{Code: "error", Msg: "Server not found"})
		}
		return utils.ErrorHandler(c, err, "Database error")
	}
	if currentType == 0 {
		return c.JSON(400, _type.H{
			Code: "invalid_server_type",
			Msg:  "SSH servers do not support Agent reinstall",
		})
	}
	if currentType != int16(mode) {
		return c.JSON(400, _type.H{
			Code: "mode_mismatch",
			Msg:  "Reinstall mode must match the server Agent type",
		})
	}
	if _, err = tx.ExecContext(ctx,
		"UPDATE servers SET type = $1, updated_at = now() WHERE id = $2 AND team_id = $3",
		mode, id, tid,
	); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	var response _type.Map

	switch mode {
	case 1:
		if _, err = tx.ExecContext(ctx, "DELETE FROM enroll_tokens WHERE server_id = $1", id); err != nil {
			return utils.ErrorHandler(c, err, "Database error")
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM agents WHERE server_id = $1", id); err != nil {
			return utils.ErrorHandler(c, err, "Database error")
		}

		agentUUID, uuidErr := uuid.NewRandom()
		if uuidErr != nil {
			return utils.ErrorHandler(c, uuidErr, "Agent ID generation error")
		}
		privateKey, publicKey, err := identity.GenerateEd25519KeyPair()
		if err != nil {
			return utils.ErrorHandler(c, err, "Key generation error")
		}

		if _, err = tx.ExecContext(ctx,
			"INSERT INTO agents (server_id, agent_uid, status, host, port, private_key) VALUES ($1, $2, $3, $4, $5, $6)",
			id, agentUUID.String(), 0, address, port, privateKey,
		); err != nil {
			return utils.ErrorHandler(c, err, "Database error")
		}

		response = _type.Map{
			"host":       address,
			"port":       port,
			"agent_uid":  agentUUID.String(),
			"public_key": base64.StdEncoding.EncodeToString([]byte(publicKey)),
		}
	case 2:
		if _, err = tx.ExecContext(ctx, "DELETE FROM agents WHERE server_id = $1", id); err != nil {
			return utils.ErrorHandler(c, err, "Database error")
		}
		dynamicConf := config.ReadDynamicConf()
		enrollToken := utils.RandomString(32)
		if enrollToken == "" {
			return utils.ErrorHandler(c, errors.New("generate enrollment token"), "Token generation error")
		}
		tokenHash := utils.SHA256(enrollToken + dynamicConf.Token)
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO enroll_tokens (server_id, token_hash, is_revoked, created_at)
			 VALUES ($1, $2, FALSE, NOW())
			 ON CONFLICT (server_id) DO UPDATE
			 SET token_hash = EXCLUDED.token_hash,
			     is_revoked = FALSE,
			     created_at = NOW()`,
			id, tokenHash,
		); err != nil {
			return utils.ErrorHandler(c, err, "Database error")
		}
		response = _type.Map{
			"hub":          dynamicConf.Domain,
			"enroll_token": enrollToken,
		}
	}

	if err = tx.Commit(); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Connection shutdown can wait for transport goroutines, so keep it outside
	// the database transaction.
	reinstallStopServer(id)
	if reconcileErr := reinstallReconcileServer(id); reconcileErr != nil {
		fmt.Println("Failed to reconcile server connection:", reconcileErr)
	}

	// Log action
	reinstallLogAdd(
		tid, uid, "server", "Reinstall Agent (ID"+strconv.FormatInt(id, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Status changed to reinstall",
		Data: response,
	})
}
