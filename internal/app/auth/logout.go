package auth

import (
	"mosona-manager/internal/_type"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func logout(c *echo.Context) error {
	sess, err := session.Get("session", c)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session error"})
	}

	for k := range sess.Values {
		delete(sess.Values, k)
	}

	if sess.Options == nil {
		sess.Options = &sessions.Options{}
	}
	sess.Options.Path = "/"
	sess.Options.MaxAge = -1
	sess.Options.HttpOnly = true
	sess.Options.SameSite = http.SameSiteLaxMode

	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return c.JSON(500, _type.H{Code: "warning", Msg: "Session save failed"})
	}

	return c.JSON(200, _type.H{Code: "ok", Msg: "Logout success"})
}
