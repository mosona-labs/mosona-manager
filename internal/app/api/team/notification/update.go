package anotification

import (
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/notification"
	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v5"
)

func update(c *echo.Context) error {
	var req []_type.TeamNotification
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, _type.H{
			Code: "invalid_request",
			Msg:  "Invalid request data",
		})
	}

	tid, _ := c.Get("tid").(int64)

	if err := db.UpdateNotificationsByTeamId(c.Request().Context(), tid, req); err != nil {
		if errors.Is(err, notification.ErrInvalidConfiguration) {
			return c.JSON(400, _type.H{Code: "invalid_notification", Msg: err.Error()})
		}
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Notifications updated successfully",
	})
}
