package amonitor

import "github.com/labstack/echo/v4"

func Router(e *echo.Group) {
	e.GET("", list)
	e.GET("/sse", sse)
	e.GET("/:id", get)
	e.GET("/:id/chart", chart)
	e.GET("/:id/realtime", realTime)
}
