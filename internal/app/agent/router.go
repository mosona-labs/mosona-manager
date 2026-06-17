package agent

import (
	"mosona-manager/internal/app/agent/middleware"
	"mosona-manager/internal/db"

	"github.com/labstack/echo/v5"
)

func Router(e *echo.Group) {
	passiveAuth := middleware.PassiveAuthWithLookup(db.GetPassiveAgentPublicKey)
	e.POST("/enroll", enroll)

	e.GET("/update/latest", updateLatest)
	e.GET("/update/download", updateDownload, passiveAuth)

	e.POST("/info", passiveInfo, passiveAuth)
	e.GET("/ws", passiveWS, passiveAuth)
	e.GET("/terminal/:session_id", terminal, passiveAuth)
}
