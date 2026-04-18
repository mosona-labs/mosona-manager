package auser

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v5"
)

func changeUsername(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)

	newUsername := c.FormValue("username")
	if newUsername == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Username cannot be empty",
		})
	}

	if err := db.UpdateUsername(uid, newUsername); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Username updated successfully",
	})
}
