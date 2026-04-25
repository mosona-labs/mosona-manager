package middleware

import (
	"mosona-manager/internal/_type"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"
)

func SameOriginWrite(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		req := c.Request()
		switch req.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return next(c)
		}

		if !sameOriginHeader(req, "Origin") {
			return c.JSON(http.StatusForbidden, _type.H{
				Code: "forbidden",
				Msg:  "Cross-origin request denied",
			})
		}
		if req.Header.Get("Origin") == "" && req.Header.Get("Referer") != "" && !sameOriginHeader(req, "Referer") {
			return c.JSON(http.StatusForbidden, _type.H{
				Code: "forbidden",
				Msg:  "Cross-origin request denied",
			})
		}

		return next(c)
	}
}

func sameOriginHeader(req *http.Request, header string) bool {
	value := strings.TrimSpace(req.Header.Get(header))
	if value == "" {
		return true
	}

	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, req.Host)
}
