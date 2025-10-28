package acategory

import (
	"errors"
	"github.com/labstack/echo/v4"
	"mosona-manager/_type"
	"mosona-manager/db"
)

func create(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	name := c.FormValue("name")

	if tid == 0 || name == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid category data",
		})
	}

	if err := db.CreateCategory(tid, name); err != nil {
		if errors.Is(err, db.ErrSameCategoryName) {
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Category name already exists",
			})
		}
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Category created",
	})
}
