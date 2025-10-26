package auth

import (
	"database/sql"
	"errors"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
	"mosona-manager/_type"
	"mosona-manager/config"
	"mosona-manager/db"
	"mosona-manager/utils"
)

func login(c echo.Context) error {
	email := c.FormValue("email")
	password := c.FormValue("password")
	rememberMe := c.FormValue("remember_me") == "true"
	otp := c.FormValue("otp")
	if email == "" || password == "" {
		return c.JSON(400, _type.H{
			Code: "warning",
			Msg:  "Email or password is empty",
		})
	}

	// Find
	user, err := db.FindUserAuthByEmail(email)
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

	// Verify
	if !user.Verified {
		return c.JSON(401, _type.H{Code: "verify", Msg: "Account is not verified"})
	}

	// TOTP
	if user.TOTP != nil && *user.TOTP != "" {
		if !totp.Validate(otp, *user.TOTP) {
			return c.JSON(401, _type.H{Code: "otp", Msg: "OTP is incorrect"})
		}
	}

	// Session
	sess, err := session.Get("session", c)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session init error"})
	}

	var maxAge = 86400 * 3 // 3 days
	if rememberMe {
		maxAge = 86400 * 30 // 30 days
	}
	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
	}
	sess.Values["uid"] = user.ID
	sess.Values["user_agent"] = c.Request().Header.Get("User-Agent")
	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session save failed"})
	}

	return c.JSON(200, _type.H{Code: "ok", Msg: "Login success"})
}
