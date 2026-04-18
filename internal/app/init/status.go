package init

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"

	"github.com/labstack/echo/v5"
)

func status(c *echo.Context) error {
	return c.JSON(200, _type.H{
		Code: "ok",
		Data: config.DynamicConf.Init,
	})
}
