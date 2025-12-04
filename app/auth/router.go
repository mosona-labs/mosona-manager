package auth

import "github.com/labstack/echo/v4"

func Router(e *echo.Group) {
	e.POST("/login", login)
	e.POST("/register", register)
	e.POST("/logout", logout)

	// OAuth
	e.GET("/oauth/:id", oauthLogin)
	e.POST("/oauth/:id", oauthCallback)

	e.GET("/keys", keys) // Key
}
