package aterminal

import "github.com/labstack/echo/v4"

func Router(e *echo.Group) {
	e.GET("", list)
	e.GET("/:id/ws", ws)
}
