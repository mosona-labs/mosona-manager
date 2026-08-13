package anotification

import (
	"mosona-manager/internal/app/middleware"

	"github.com/labstack/echo/v5"
)

func Router(e *echo.Group) {
	e.GET("", list)
	e.PUT("", update, middleware.WriteAuth)
	e.POST("/validate", validate, middleware.WriteAuth)
	e.POST("/test", test, middleware.WriteAuth)
}
