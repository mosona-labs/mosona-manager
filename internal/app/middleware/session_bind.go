package middleware

import (
	"mosona-manager/internal/config"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo/v5"
)

func SessionBindingOK(c *echo.Context, sess *sessions.Session) bool {
	reqUA := c.Request().Header.Get("User-Agent")
	userAgent := sess.Values["user_agent"]
	if userAgent == nil || userAgent != reqUA {
		return false
	}

	if !config.DynamicConf.SessionBindIP {
		return true
	}

	storedIP, ok := sess.Values["client_ip"].(string)
	if !ok || storedIP == "" {
		return true
	}

	return storedIP == c.RealIP()
}
