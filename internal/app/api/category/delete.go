package acategory

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v5"
)

func del(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	uid, _ := c.Get("uid").(int64)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	if tid == 0 || id == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid category data",
		})
	}

	cat, err := db.GetCategoryById(tid, id)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	if err := db.DeleteCategory(tid, id); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Log action
	influx.LogAdd(
		tid, uid, "category", "Delete Category: "+cat.Name+" (ID"+strconv.FormatInt(cat.ID, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "low",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Category deleted",
	})
}
