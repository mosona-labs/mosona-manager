package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/app/middleware"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/security/passwordhash"
	"strings"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func login(c *echo.Context) error {
	email := c.FormValue("email")
	password := c.FormValue("password")
	rememberMe := c.FormValue("remember_me") == "true"
	emailKey := strings.ToLower(strings.TrimSpace(email))
	ip := c.RealIP()
	if email == "" || password == "" {
		return c.JSON(400, _type.H{
			Code: "warning",
			Msg:  "Email or password is empty",
		})
	}
	if retryIn, err := checkLoginRateLimit(c.Request().Context(), emailKey, ip); err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Rate limit check failed"})
	} else if retryIn > 0 {
		return c.JSON(429, _type.H{
			Code: "rate_limited",
			Msg:  fmt.Sprintf("Too many failed login attempts. Try again in %d seconds", int(retryIn.Seconds())+1),
		})
	}

	// Find
	user, err := db.GetUserAuthByEmail(email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = recordLoginFailure(c.Request().Context(), emailKey, ip)
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Email or password is incorrect",
			})
		} else {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Database error",
			})
		}
	}
	ok, needsRehash, err := passwordhash.Verify(
		password,
		user.Password,
		user.Salt,
		config.DynamicConf.Token,
	)
	if err != nil || !ok {
		_ = recordLoginFailure(c.Request().Context(), emailKey, ip)
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Email or password is incorrect",
		})
	}
	if needsRehash {
		newHash, hashErr := passwordhash.Hash(password)
		if hashErr != nil {
			log.Printf("password hash migration failed for user id %d: %v", user.ID, hashErr)
		} else if _, execErr := db.Db.ExecContext(
			c.Request().Context(),
			"UPDATE users SET password = $1 WHERE id = $2 AND password = $3",
			newHash, user.ID, user.Password,
		); execErr != nil {
			log.Printf("password hash migration update failed for user id %d: %v", user.ID, execErr)
		} else {
			log.Printf("password hash migrated for user id %d", user.ID)
		}
	}
	clearLoginFailures(c.Request().Context(), emailKey, ip)

	// Session
	sess, err := session.Get("session", c)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session init error"})
	}

	// Already logged in
	if sess.Values["uid"] != nil {
		currentUID, ok := sess.Values["uid"].(int64)
		if ok && currentUID > 0 && middleware.SessionBindingOK(c, sess) {
			return c.JSON(500, _type.H{Code: "error", Msg: "Already logged in"})
		}
	}

	var maxAge = 86400 * 3 // 3 days
	if rememberMe {
		maxAge = 86400 * 30 // 30 days
	}
	sess.Options = sessionOptions(maxAge)

	if (user.TOTP != nil && *user.TOTP != "") || config.DynamicConf.EmailVerifyLogin || !user.Verified {
		sess.Values["pre_2fa_uid"] = user.ID
		if err = sess.Save(c.Request(), c.Response()); err != nil {
			return c.JSON(500, _type.H{Code: "error", Msg: "Session save failed"})
		}
		// 2FA Required
		return c.JSON(200, _type.H{Code: "2fa_required", Msg: "Two-factor authentication required"})
	} else {
		_, err := finalizeAuthenticatedSession(c, user.ID, maxAge)
		if err != nil {
			return c.JSON(500, _type.H{Code: "error", Msg: "Session save failed"})
		}
		loginEvent(user.ID, c.RealIP(), c.Request().Header.Get("User-Agent"), user.IsAdmin)
		return c.JSON(200, _type.H{Code: "ok", Msg: "Login success"})
	}
}
