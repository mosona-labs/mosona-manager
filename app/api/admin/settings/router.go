package msettings

import "github.com/labstack/echo/v4"

func Router(e *echo.Group) {
	e.GET("", get)
	e.POST("", set)
}
