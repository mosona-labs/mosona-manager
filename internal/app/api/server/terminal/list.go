package aterminal

import (
	"mosona-manager/internal/db"
	"mosona-manager/pkg/_type"

	"github.com/labstack/echo/v4"
)

func list(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)

	servers, err := db.ListTerminals(tid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to list terminal servers",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: servers,
	})
}
