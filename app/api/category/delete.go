package acategory

import (
	"github.com/labstack/echo/v4"
	"mosona-manager/_type"
	"mosona-manager/db"
	"strconv"
)

func del(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	if tid == 0 || id == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid category data",
		})
	}

	if err := db.DeleteCategory(tid, id); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Category deleted",
	})
}
