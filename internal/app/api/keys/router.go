package akeys

import "github.com/labstack/echo/v5"

func Router(e *echo.Group) {
	e.GET("", list)
	e.POST("", add)
	e.PUT("/:id", edit)
	e.DELETE("/:id", del)
}
