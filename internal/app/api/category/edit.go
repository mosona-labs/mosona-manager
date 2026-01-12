package acategory

import (
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v4"
)

func edit(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	uid, _ := c.Get("uid").(int64)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	name := c.FormValue("name")

	if tid == 0 || id == 0 || name == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid category data",
		})
	}

	if err := db.EditCategory(tid, id, name); err != nil {
		if errors.Is(err, db.ErrSameCategoryName) {
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Category name already exists",
			})
		}
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Log action
	influx.LogAdd(
		tid, uid, "category", "Edit Category: "+name+" (ID"+strconv.FormatInt(id, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "low",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Category updated",
	})
}
