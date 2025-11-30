package admin

import (
	muser "mosona-manager/app/api/admin/user"

	"github.com/labstack/echo/v4"
)

func Router(e *echo.Group) {
	e.GET("/dashboard", dashboard)

	muser.Router(e.Group("/users")) // Users
}
