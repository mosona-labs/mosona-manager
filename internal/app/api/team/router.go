package ateam

import (
	anotification "mosona-manager/internal/app/api/team/notification"
	"mosona-manager/internal/app/middleware"

	"github.com/labstack/echo/v5"
)

func Router(e *echo.Group) {
	e.GET("", info)
	e.POST("", create)
	e.PUT("/:id", edit, middleware.WriteAuth)
	e.DELETE("/leave/:id", leave)
	e.GET("/public-page", getPublicPage)
	e.PUT("/public-page", setPublicPage, middleware.WriteAuth)

	anotification.Router(e.Group("/notification"))
}
