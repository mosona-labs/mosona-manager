package auth

import (
	"mosona-manager/internal/_type"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

func logout(c echo.Context) error {
	sess, err := session.Get("session", c)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session error"})
	}

	sess.Options.MaxAge = -1
	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return c.JSON(500, _type.H{Code: "warning", Msg: "Session save failed"})
	}

	return c.JSON(200, _type.H{Code: "ok", Msg: "Logout success"})
}
