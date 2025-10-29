package aserver

import (
	"github.com/labstack/echo/v4"
	"mosona-manager/_type"
	"mosona-manager/db"
	"strconv"
)

func category(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	sid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	categoryId, _ := strconv.ParseInt(c.FormValue("category_id"), 10, 64)

	if tid == 0 || sid == 0 || categoryId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid server category data",
		})
	}

	if err := db.SetServerCategory(tid, sid, categoryId); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Server category updated",
	})
}
