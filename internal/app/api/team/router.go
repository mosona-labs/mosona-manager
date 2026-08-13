package ateam

import (
	anotification "mosona-manager/internal/app/api/team/notification"
	"mosona-manager/internal/app/middleware"

	"github.com/labstack/echo/v5"
)

func Router(e *echo.Group) {
	team := e.Group("", middleware.TeamAccess)
	team.GET("", info)
	e.POST("", create, middleware.AvatarUploadLimit)
	team.PUT("/:id", edit, middleware.WriteAuth, middleware.AvatarUploadLimit)
	e.DELETE("/leave/:id", leave)
	team.GET("/public-page", getPublicPage)
	team.PUT("/public-page", setPublicPage, middleware.WriteAuth)
	team.POST("/export", exportTeam, middleware.WriteAuth)
	team.POST("/import", importTeam, middleware.WriteAuth)

	anotification.Router(team.Group("/notification"))
}
