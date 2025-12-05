package auser

import "github.com/labstack/echo/v4"

func Router(e *echo.Group) {
	e.GET("/me", me)
	e.POST("/find", find)

	// Info
	e.PUT("/edit/username", changeUsername)

	// Switch Team
	e.POST("/config/active-team/:id", setActiveTeam)

	// Sessions
	e.GET("/sessions", sessions)
	e.DELETE("/sessions/:sid", sessionRevoke)
	e.DELETE("/sessions", sessionRevokeAll)

	// OAuth
	e.GET("/oauth", oauthIdentities)
	e.DELETE("/oauth/:id", oauthRevoke)
	e.POST("/oauth/:id", oauthLink)
}
