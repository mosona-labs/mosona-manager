package anotification

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"

	"github.com/labstack/echo/v5"
	"github.com/nicholas-fedor/shoutrrr"
)

func test(c *echo.Context) error {
	uri := c.FormValue("uri")

	if err := shoutrrr.Send(uri, "Test notification from Mosona Manager\n\n"+config.DynamicConf.Domain); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  err.Error(),
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Test notification sent successfully",
	})
}
