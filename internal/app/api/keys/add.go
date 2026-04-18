package akeys

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v5"
)

func add(c *echo.Context) error {
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
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
	})
}
