package auser

import "github.com/labstack/echo/v4"

func Router(e *echo.Group) {
	e.GET("/me", me)
	e.POST("/find", find)

	e.POST("/config/active-team/:id", setActiveTeam)
}
