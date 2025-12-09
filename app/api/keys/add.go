package akeys

import (
	"mosona-manager/_type"
	"mosona-manager/db"

	"github.com/labstack/echo/v4"
)

func add(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	if tid == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "No Active Team ID",
		})
	}

	name := c.FormValue("name")
	content := c.FormValue("content")
	password := c.FormValue("password")
	if name == "" || content == "" {
		return c.JSON(400, _type.H{
			Code: "input",
			Msg:  "Name and Content cannot be empty",
		})
	}

	if _, err := db.AddKey(tid, name, content, password); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
	})
}
