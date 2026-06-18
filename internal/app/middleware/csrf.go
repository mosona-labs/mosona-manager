package middleware

import (
	"mosona-manager/internal/_type"
	"mosona-manager/pkg/httporigin"
	"net/http"

	"github.com/labstack/echo/v5"
)

func SameOriginWrite(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		req := c.Request()
		switch req.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return next(c)
		}

		if !httporigin.SameOriginHeader(req, "Origin") {
			return c.JSON(http.StatusForbidden, _type.H{
				Code: "forbidden",
				Msg:  "Cross-origin request denied",
			})
		}
		if req.Header.Get("Origin") == "" && req.Header.Get("Referer") != "" && !httporigin.SameOriginHeader(req, "Referer") {
			return c.JSON(http.StatusForbidden, _type.H{
				Code: "forbidden",
				Msg:  "Cross-origin request denied",
			})
		}

		return next(c)
	}
}
