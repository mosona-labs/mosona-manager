package init

import "github.com/labstack/echo/v4"

func Router(e *echo.Group) {
	e.GET("", status)
	e.POST("", initialize)
}
