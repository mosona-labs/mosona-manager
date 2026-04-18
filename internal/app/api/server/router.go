package aserver

import (
	"mosona-manager/internal/app/api/server/monitor"
	"mosona-manager/internal/app/api/server/terminal"
	"mosona-manager/internal/app/middleware"

	"github.com/labstack/echo/v5"
)

func Router(e *echo.Group) {
	e.GET("/:id", info)
	e.PUT("/:id", edit, middleware.WriteAuth)
	e.POST("", add, middleware.WriteAuth)
	e.DELETE("/:id", del, middleware.WriteAuth)

	e.POST("/:id/reinstall", reinstall, middleware.WriteAuth)
	e.PUT("/:id/category", category, middleware.WriteAuth)

	// Monitor
	amonitor.Router(e.Group("/monitor"))
	// Terminal
	aterminal.Router(e.Group("/terminal"))
}
