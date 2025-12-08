package akeys

import (
	"mosona-manager/_type"
	"mosona-manager/db"
	"strconv"

	"github.com/labstack/echo/v4"
)

func edit(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	if tid == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid team data",
		})
	}

	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid key ID",
		})
	}

	name := c.FormValue("name")
	if name == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Key name cannot be empty",
		})
	}

	if err := db.UpdateKey(tid, id, name); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Key updated successfully",
	})
}
