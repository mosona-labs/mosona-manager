package akeys

import (
	"mosona-manager/_type"
	"mosona-manager/db"

	"github.com/labstack/echo/v4"
)

func list(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	if tid == 0 {
		return c.JSON(400, _type.H{
			Code: "team",
			Msg:  "No Active Team ID",
		})
	}

	data, err := db.GetKeysByTeamID(tid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: data,
	})
}
