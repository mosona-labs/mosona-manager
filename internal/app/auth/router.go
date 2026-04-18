package auth

import (
	"github.com/labstack/echo/v5"
)

func Router(e *echo.Group) {
	e.POST("/login", login)
	e.POST("/register", register)
	e.POST("/logout", logout)

	// OAuth
	e.GET("/oauth/:id", oauthLogin)
	e.POST("/oauth/:id", oauthCallback)

	// 2FA
	tfa := e.Group("/2fa", middlewareFA)
	tfa.GET("/status", getTwoFAStatus)
	tfa.POST("/send_code", sendMFACode)
	tfa.POST("/verify_code", verifyMFACode)
	tfa.POST("/verify_totp", verifyTOTP)

	e.GET("/keys", keys) // Key
}
