package aserver

import (
	"github.com/labstack/echo/v4"
	amonitor "mosona-manager/app/api/server/monitor"
)

func Router(e *echo.Group) {
	e.POST("", add)
	e.PUT("/:id/category", category)

	// Monitor
	amonitor.Router(e.Group("/monitor"))
}
