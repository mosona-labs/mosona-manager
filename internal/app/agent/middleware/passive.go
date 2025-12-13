package middleware

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"

	"github.com/labstack/echo/v4"
)

func PassiveAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		uid := c.Request().Header.Get("X-Agent-Id")
		ts := c.Request().Header.Get("X-Agent-Timestamp")
		nonce := c.Request().Header.Get("X-Agent-Nonce")
		signature := c.Request().Header.Get("X-Agent-Signature")
		if uid == "" || ts == "" || nonce == "" || signature == "" {
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
		sig, err := base64.StdEncoding.DecodeString(signature)
		if err != nil {
			return c.JSON(400, _type.H{
				Code: "unauthorized",
				Msg:  "Invalid signature encoding",
			})
		}
		if !ed25519.Verify(pubKey, []byte(fmt.Sprintf("%s\n%s\n%s", uid, ts, nonce)), sig) {
			return c.JSON(400, _type.H{
				Code: "unauthorized",
				Msg:  "Signature verification failed",
			})
		}

		c.Set("server_id", serverId)

		return next(c)
	}
}
