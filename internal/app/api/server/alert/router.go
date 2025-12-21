package aalert

import (
	"mosona-manager/internal/app/middleware"

	"github.com/labstack/echo/v4"
)

func Router(e *echo.Group) {
	e.GET("", list)
	e.PUT("/:id", set, middleware.WriteAuth)
	e.DELETE("/:item/:id", del, middleware.WriteAuth)
}
