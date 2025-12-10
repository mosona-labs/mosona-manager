package muser

import "github.com/labstack/echo/v4"

func Router(e *echo.Group) {
	e.GET("/list", list)
	e.POST("", add)
	e.PUT("/:id", edit)
	e.DELETE("/:id", del)
}
