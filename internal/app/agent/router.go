package agent

import (
	"mosona-manager/internal/app/agent/middleware"

	"github.com/labstack/echo/v4"
)

func Router(e *echo.Group) {
	e.POST("/enroll", enroll)

	e.POST("/info", passiveInfo, middleware.PassiveAuth)
	e.GET("/ws", passiveWS, middleware.PassiveAuth)
}
