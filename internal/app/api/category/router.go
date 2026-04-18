package acategory

import "github.com/labstack/echo/v5"

func Router(e *echo.Group) {
	e.GET("", list)
	e.POST("", create)
	e.PUT("/:id", edit)
	e.DELETE("/:id", del)
	e.PUT("/sort", sort)
}
