package acategory

import (
	"mosona-manager/internal/app/middleware"

	"github.com/labstack/echo/v5"
)

func Router(e *echo.Group) {
	e.GET("", list)
	e.POST("", create, middleware.WriteAuth)
	e.PUT("/:id", edit, middleware.WriteAuth)
	e.DELETE("/:id", del, middleware.WriteAuth)
	e.PUT("/sort", sort, middleware.WriteAuth)
}
