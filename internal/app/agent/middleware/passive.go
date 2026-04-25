package middleware

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/pem"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/pkg/identity"
	"time"

	"github.com/labstack/echo/v5"
)

func PassiveAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		uid := c.Request().Header.Get("X-Agent-Id")
		ts := c.Request().Header.Get("X-Agent-Timestamp")
		nonce := c.Request().Header.Get("X-Agent-Nonce")
		signature := c.Request().Header.Get("X-Agent-Signature")
		if uid == "" || ts == "" || len(nonce) < 16 || signature == "" {
			return c.JSON(400, _type.H{
				Code: "unauthorized",
				Msg:  "Missing authentication headers",
			})
		}
		serverId, publicKey, err := db.GetPassiveAgentPublicKey(uid)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return c.JSON(400, _type.H{
					Code: "unauthorized",
					Msg:  "Agent not registered",
				})
			}
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Database error",
			})
		}
		block, _ := pem.Decode([]byte(publicKey))
		if block == nil {
			return c.JSON(400, _type.H{
				Code: "unauthorized",
				Msg:  "Invalid public key",
			})
		}
		pubKey := ed25519.PublicKey(block.Bytes)
		if err := identity.VerifySignedHeaders(pubKey, uid, ts, nonce, signature, time.Now()); err != nil {
			return c.JSON(400, _type.H{
				Code: "unauthorized",
				Msg:  err.Error(),
			})
		}

		c.Set("server_id", serverId)

		return next(c)
	}
}
