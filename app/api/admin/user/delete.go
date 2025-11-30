package muser

import (
	"mosona-manager/_type"
	"mosona-manager/db"
	"strconv"

	"github.com/labstack/echo/v4"
)

func del(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid user ID",
		})
	}

	if _, err := db.Db.Exec("DELETE FROM users WHERE id = $1", id); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "User deleted successfully",
	})
}
