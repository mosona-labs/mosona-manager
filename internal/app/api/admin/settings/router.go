package msettings

import (
	"mosona-manager/internal/app/middleware"

	"github.com/labstack/echo/v5"
)

func Router(e *echo.Group) {
	e.GET("", get)
	e.POST("", set)
	e.POST("/favicon", uploadFavicon, middleware.AvatarUploadLimit)

	// Test
	e.POST("/test/email", testEmail)
}
