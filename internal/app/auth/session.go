package auth

import (
	"mosona-manager/internal/_type"

	"github.com/labstack/echo/v5"
)

func loginSession(c *echo.Context, uid int64, isAdmin bool) *_type.H {
	sessID, err := finalizeAuthenticatedSession(c, uid, 86400*3)
	if err != nil {
		return &_type.H{Code: "error", Msg: "Session save failed"}
	}

	loginEvent(uid, sessID, c.RealIP(), c.Request().Header.Get("User-Agent"), isAdmin)

	return nil
}
