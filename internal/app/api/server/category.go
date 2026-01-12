package aserver

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v4"
)

func category(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	uid, _ := c.Get("uid").(int64)
	sid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	categoryId, _ := strconv.ParseInt(c.FormValue("category_id"), 10, 64)

	if tid == 0 || sid == 0 || categoryId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid server category data",
		})
	}

	// Get Info
	categoryInfo, err := db.GetCategoryById(tid, categoryId)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	if categoryInfo.ID == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Category does not exist",
		})
	}

	// Set Category
	if err = db.SetServerCategory(tid, sid, categoryId); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Log action
	influx.LogAdd(
		tid, uid, "server", "Set Server (ID"+strconv.FormatInt(sid, 10)+") Category to "+categoryInfo.Name,
		c.RealIP(), c.Request().UserAgent(), "low",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Server category updated",
	})
}
