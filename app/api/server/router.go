package aserver

import (
	amonitor "mosona-manager/app/api/server/monitor"
	aterminal "mosona-manager/app/api/server/terminal"

	"github.com/labstack/echo/v4"
)

func Router(e *echo.Group) {
	e.GET("/:id", info)
	e.PUT("/:id", edit)
	e.POST("", add)
	e.PUT("/:id/category", category)

	// Monitor
	amonitor.Router(e.Group("/monitor"))
	// Terminal
	aterminal.Router(e.Group("/terminal"))
}
