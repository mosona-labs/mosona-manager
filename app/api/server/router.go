package aserver

import (
	amonitor "mosona-manager/app/api/server/monitor"
	aterminal "mosona-manager/app/api/server/terminal"
	"mosona-manager/app/middleware"

	"github.com/labstack/echo/v4"
)

func Router(e *echo.Group) {
	e.GET("/:id", info)
	e.PUT("/:id", edit, middleware.WriteAuth)
	e.POST("", add, middleware.WriteAuth)
	e.PUT("/:id/category", category, middleware.WriteAuth)

	// Monitor
	amonitor.Router(e.Group("/monitor"))
	// Terminal
	aterminal.Router(e.Group("/terminal"))
}
