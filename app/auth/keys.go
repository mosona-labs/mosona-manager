package auth

import (
	"mosona-manager/_type"
	"mosona-manager/config"

	"github.com/labstack/echo/v4"
)

func keys(c echo.Context) error {
	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"captcha": config.DynamicConf.CaptchaSiteKey,
		},
	})
}
