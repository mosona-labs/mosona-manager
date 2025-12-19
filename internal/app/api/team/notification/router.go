package anotification

import "github.com/labstack/echo/v4"

func Router(e *echo.Group) {
	e.GET("", list)
	e.PUT("", update)
	e.POST("/test", test)
}
