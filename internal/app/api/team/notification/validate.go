package anotification

import (
	"errors"

	"mosona-manager/internal/_type"
	"mosona-manager/internal/notification"

	"github.com/labstack/echo/v5"
)

func validate(c *echo.Context) error {
	var entry _type.TeamNotification
	if err := c.Bind(&entry); err != nil {
		return c.JSON(400, _type.H{Code: "invalid_request", Msg: "Invalid request data"})
	}
	if _, err := notification.NormalizeEntry(c.Request().Context(), entry); err != nil {
		if errors.Is(err, notification.ErrInvalidConfiguration) {
			return c.JSON(400, _type.H{Code: "invalid_notification", Msg: err.Error()})
		}
		return err
	}
	return c.JSON(200, _type.H{Code: "ok", Msg: "Notification configuration is valid"})
}
