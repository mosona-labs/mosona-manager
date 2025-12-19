package ateam

import (
	anotification "mosona-manager/internal/app/api/team/notification"
	"mosona-manager/internal/app/middleware"

	"github.com/labstack/echo/v4"
)

func Router(e *echo.Group) {
	e.GET("", info)
	e.POST("", create)
	e.PUT("/:id", edit, middleware.WriteAuth)
	e.DELETE("/leave/:id", leave)

	anotification.Router(e.Group("/notification"))
}
