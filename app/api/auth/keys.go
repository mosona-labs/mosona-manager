package auth

import (
	"github.com/labstack/echo/v4"
	"mosona-manager/_type"
	"mosona-manager/config"
)

func keys(c echo.Context) error {
	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"captcha": config.DynamicConf.CaptchaSiteKey,
			"google":  config.DynamicConf.GoogleClientID,
		},
	})
}
