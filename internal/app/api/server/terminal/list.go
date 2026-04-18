package aterminal

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v5"
)

func list(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)

	servers, err := db.ListTerminals(tid)
	if err != nil {
		return utils.ErrorHandler(c, err, "Failed to list terminal servers")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: servers,
	})
}
