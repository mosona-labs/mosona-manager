package init

import (
	"mosona-manager/internal/app/middleware"

	"github.com/labstack/echo/v5"
)

func Router(e *echo.Group) {
	e.GET("", status)
	e.POST("", initialize, middleware.InitAuth)
}
