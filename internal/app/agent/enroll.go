package agent

import (
	"database/sql"
	"errors"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/app/auth"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func enroll(c *echo.Context) error {
	ip := c.RealIP()
	ctx := c.Request().Context()
	if retryIn, err := auth.CheckEnrollRateLimit(ctx, ip); err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Rate limit check failed"})
	} else if retryIn > 0 {
		return c.JSON(429, _type.H{
			Code: "rate_limited",
			Msg:  fmt.Sprintf("Too many enroll attempts. Try again in %d seconds", int(retryIn.Seconds())+1),
		})
	}

	token := c.FormValue("token")
	publicKey := c.FormValue("public_key")
	version := c.FormValue("version")

	if token == "" || publicKey == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid enroll data",
		})
	}

	if _, err := utils.ParseAgentEd25519PublicKeyPEM(publicKey); err != nil {
		_ = auth.RecordEnrollFailure(ctx, ip)
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid public key",
		})
	}

	tokenHash := utils.SHA256(token + config.ReadDynamicConf().Token)
	serverId, err := db.GetEnrollToken(tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = auth.RecordEnrollFailure(ctx, ip)
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Enroll token is invalid",
			})
		}
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	if serverId == 0 {
		_ = auth.RecordEnrollFailure(ctx, ip)
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Enroll token is invalid",
		})
	}

	agentUID, err := uuid.NewRandom()
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to generate agent identity",
		})
	}

	tx, err := db.Db.Begin()
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.Exec(
		"INSERT INTO agents (server_id, agent_uid, status, last_ip, last_version, public_key) VALUES ($1, $2, $3, $4, $5, $6)",
		serverId, agentUID.String(), 1, c.RealIP(), version, publicKey,
	); err != nil {
		_ = tx.Rollback()
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	if _, err = tx.Exec(
		"UPDATE enroll_tokens SET is_revoked = TRUE WHERE token_hash = $1",
		tokenHash,
	); err != nil {
		_ = tx.Rollback()
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	if err = tx.Commit(); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Enroll successful",
		Data: agentUID.String(),
	})
}
