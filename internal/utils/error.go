package utils

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"

	"github.com/labstack/echo/v4"
)

func ErrorHandler(c echo.Context, err error, msg string) error {
	debugMode := config.ReadDynamicConf().Debug

	body := _type.H{
		Code: "error",
		Msg:  msg,
	}
	if debugMode && err != nil {
		body.Msg = msg + ": " + err.Error()
	}
	return c.JSON(500, body)
}
