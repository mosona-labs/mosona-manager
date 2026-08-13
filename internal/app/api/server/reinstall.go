package aserver

import (
	"encoding/base64"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/connect/conn"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"mosona-manager/pkg/identity"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func reinstall(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	tid, _ := c.Get("tid").(int64)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid server ID",
		})
	}
	mode, _ := strconv.Atoi(c.FormValue("mode"))

	// Exists
	exist, err := db.IsServerExists(tid, id)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	if !exist {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Server not found",
		})
	}

	tx, err := db.Db.Begin()
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	defer func() { _ = tx.Rollback() }()

	var response _type.Map

	switch mode {
	case 0:
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "SSH Mode don't need to reinstall",
		})
	case 1:
		address := c.FormValue("address")
		port, _ := strconv.Atoi(c.FormValue("port"))

		if _, err = tx.Exec("DELETE FROM agents WHERE server_id = $1", id); err != nil {
			_ = tx.Rollback()
			return utils.ErrorHandler(c, err, "Database error")
		}

		agentUUID, _ := uuid.NewUUID()
		privateKey, publicKey, err := identity.GenerateEd25519KeyPair()
		if err != nil {
			return utils.ErrorHandler(c, err, "Key generation error")
		}

		if _, err = tx.Exec(
			"INSERT INTO agents (server_id, agent_uid, status, host, port, private_key) VALUES ($1, $2, $3, $4, $5, $6)",
			id, agentUUID.String(), 0, address, port, privateKey,
		); err != nil {
			_ = tx.Rollback()
			return utils.ErrorHandler(c, err, "Database error")
		}

		response = _type.Map{
			"host":       address,
			"port":       port,
			"agent_uid":  agentUUID.String(),
			"public_key": base64.StdEncoding.EncodeToString([]byte(publicKey)),
		}
	case 2:
		if _, err = tx.Exec("DELETE FROM agents WHERE server_id = $1", id); err != nil {
			_ = tx.Rollback()
			return utils.ErrorHandler(c, err, "Database error")
		}
		enrollToken := utils.RandomString(32)
		tokenHash := utils.SHA256(enrollToken + config.DynamicConf.Token)
		if _, err = tx.Exec(
			`INSERT INTO enroll_tokens (server_id, token_hash, is_revoked, created_at)
			 VALUES ($1, $2, FALSE, NOW())
			 ON CONFLICT (server_id) DO UPDATE
			 SET token_hash = EXCLUDED.token_hash,
			     is_revoked = FALSE,
			     created_at = NOW()`,
			id, tokenHash,
		); err != nil {
			_ = tx.Rollback()
			return utils.ErrorHandler(c, err, "Database error")
		}
		response = _type.Map{
			"hub":          config.DynamicConf.Domain,
			"enroll_token": enrollToken,
		}
	}

	if err = tx.Commit(); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	go func() {
		if err = conn.StartServer(id, int16(mode)); err != nil {
			fmt.Println("Failed to start server connection:", err)
		}
	}()

	// Log action
	influx.LogAdd(
		tid, uid, "server", "Reinstall Agent (ID"+strconv.FormatInt(id, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Status changed to reinstall",
		Data: response,
	})
}
