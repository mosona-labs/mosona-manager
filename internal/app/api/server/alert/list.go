package aalert

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v5"
)

func list(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)

	alerts, err := db.GetServerAlertsByTeamId(tid)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	teamAlerts, err := db.GetTeamAlertsByTeamId(tid)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: _type.Map{
			"alerts":      alerts,
			"team_alerts": teamAlerts,
		},
	})
}
