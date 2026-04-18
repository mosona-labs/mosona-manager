package mteam

import "github.com/labstack/echo/v5"

func Router(e *echo.Group) {
	e.GET("/list", list)
}
