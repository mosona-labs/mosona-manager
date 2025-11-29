package ateam

import (
	"mosona-manager/app/middleware"

	"github.com/labstack/echo/v4"
)

func Router(e *echo.Group) {
	e.GET("", info)
	e.POST("", create)
	e.PUT("/:id", edit, middleware.WriteAuth)
	e.DELETE("/leave/:id", leave)

	// Plans
	e.GET("/plans", plans)
}
