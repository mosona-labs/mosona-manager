package acategory

import (
	"mosona-manager/_type"
	"mosona-manager/db"

	"github.com/labstack/echo/v4"
)

func sort(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	var ids []int64
	if err := c.Bind(&ids); err != nil {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid category data",
		})
	}

	if tid == 0 || len(ids) == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid category data",
		})
	}

	if err := db.SortCategories(tid, ids); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Categories sorted",
	})
}
