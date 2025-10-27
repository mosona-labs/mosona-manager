package ateam

import "github.com/labstack/echo/v4"

func Router(e *echo.Group) {
	e.POST("", create)

	// Plans
	e.GET("/plans", plans)
}
