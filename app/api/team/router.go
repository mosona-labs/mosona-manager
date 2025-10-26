package ateam

import "github.com/labstack/echo/v4"

func Router(e *echo.Group) {
	e.GET("/plans", plans)
	e.POST("/plans", create)
}
