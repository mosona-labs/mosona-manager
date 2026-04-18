package moauth

import "github.com/labstack/echo/v5"

func Router(e *echo.Group) {
	e.GET("", list)
	e.POST("", add)
	e.PUT("/:id", update)
	e.DELETE("/:id", del)

	e.POST("/sort", sort)
}
