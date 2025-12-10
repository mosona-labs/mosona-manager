package aterminal

import (
	"mosona-manager/internal/app/middleware"

	"github.com/labstack/echo/v4"
)

func Router(e *echo.Group) {
	e.GET("", list)
	e.GET("/:id/ws", ws, middleware.TerminalAuth)
}
