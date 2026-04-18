package alogs

import "github.com/labstack/echo/v5"

func Router(e *echo.Group) {
	e.GET("", list)
}
