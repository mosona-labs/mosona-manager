package auth

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"time"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func login(c *echo.Context) error {
	email := c.FormValue("email")
	password := c.FormValue("password")
	rememberMe := c.FormValue("remember_me") == "true"
	if email == "" || password == "" {
		return c.JSON(400, _type.H{
			Code: "warning",
			Msg:  "Email or password is empty",
		})
	}

	// Find
	user, err := db.GetUserAuthByEmail(email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
	if utils.SHA256(password+user.Salt+config.DynamicConf.Token) != user.Password {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Email or password is incorrect",
		})
	}

	// Session
	sess, err := session.Get("session", c)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session init error"})
	}

	// Already logged in
	if sess.Values["uid"] != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Already logged in"})
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
		// Active Team
		activeTid, err := db.GetUserActiveTeam(user.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
		}
		sess.Values["uid"] = user.ID
		sess.Values["tid"] = activeTid
		sess.Values["user_agent"] = c.Request().Header.Get("User-Agent")
		sess.Values["time"] = time.Now().Unix()
		if err = sess.Save(c.Request(), c.Response()); err != nil {
			return c.JSON(500, _type.H{Code: "error", Msg: "Session save failed"})
		}
	}

	loginEvent(user.ID, sess.ID, c.RealIP(), c.Request().Header.Get("User-Agent"), user.IsAdmin)

	return c.JSON(200, _type.H{Code: "ok", Msg: "Login success"})
}
