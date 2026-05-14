package msettings

import "github.com/labstack/echo/v5"

func Router(e *echo.Group) {
	e.GET("", get)
	e.POST("", set)
	e.POST("/favicon", uploadFavicon)

	// Test
	e.POST("/test/email", testEmail)
}
