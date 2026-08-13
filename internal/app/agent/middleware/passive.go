package middleware

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/utils"
	"mosona-manager/pkg/identity"
	"time"

	"github.com/labstack/echo/v5"
)

func PassiveAuthWithLookup(lookup func(agentUID string) (int64, string, error)) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return passiveAuthWithLookup(next, lookup)
	}
}

func passiveAuthWithLookup(next echo.HandlerFunc, lookup func(agentUID string) (int64, string, error)) echo.HandlerFunc {
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
		if lookup == nil {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Agent authentication is not configured",
			})
		}
		serverId, publicKey, err := lookup(uid)
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
		pubKey, err := utils.ParseAgentEd25519PublicKeyPEM(publicKey)
		if err != nil {
			return c.JSON(400, _type.H{
				Code: "unauthorized",
				Msg:  "Invalid public key",
			})
		}
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
