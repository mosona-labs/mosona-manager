package middleware

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/siteaccess"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

func RequireUserBaseHost(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if c.Request().URL.Path == "/health" {
			return next(c)
		}

		baseURL := strings.TrimSpace(config.ReadDynamicConf().Domain)
		if baseURL == "" {
			return next(c)
		}

		if siteaccess.HostAllowed(c.Request().Host, baseURL) {
			return next(c)
		}

		return c.JSON(http.StatusForbidden, _type.H{
			Code: "forbidden",
			Msg:  "Request host does not match configured site URL",
		})
	}
}

func RestrictPublicPageHost(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if !siteaccess.IsPublicPageHost(c.Request().Host) {
			return next(c)
		}
		if siteaccess.PublicPagePathAllowed(c.Request().URL.Path) {
			return next(c)
		}
		return c.JSON(http.StatusForbidden, _type.H{
			Code: "forbidden",
			Msg:  "This host only serves public status pages",
		})
	}
}