package admin

import (
	"mosona-manager/internal/app/api/admin/oauth"
	"mosona-manager/internal/app/api/admin/settings"
	"mosona-manager/internal/app/api/admin/user"

	"github.com/labstack/echo/v4"
)

func Router(e *echo.Group) {
	e.GET("/dashboard", dashboard)

	muser.Router(e.Group("/users"))        // Users
	msettings.Router(e.Group("/settings")) // Settings
	moauth.Router(e.Group("/oauth"))       // OAuth
}
