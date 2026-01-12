package aalert

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v4"
)

func del(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id == 0 {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid alert ID",
		})
	}

	item := c.Param("item")

	override := c.QueryParam("override") == "true"

	// Delete alert
	var err error
	var affected int64 = 1
	if id > 0 {
		err = db.DeleteServerAlert(tid, id, item)
	} else {
		affected, err = db.DeleteTeamAlert(tid, item, override)
	}
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Alert deleted successfully",
		Data: affected,
	})
}
