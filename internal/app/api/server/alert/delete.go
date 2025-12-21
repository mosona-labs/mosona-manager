package aalert

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"strconv"

	"github.com/labstack/echo/v4"
)

func del(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	item := c.Param("item")

	if err := db.DeleteServerAlert(tid, id, item); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Alert deleted successfully",
	})
}
