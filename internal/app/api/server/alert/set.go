package aalert

import (
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v4"
)

func set(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id == 0 {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid alert ID",
		})
	}

	item := c.FormValue("item")
	threshold, _ := strconv.Atoi(c.FormValue("threshold"))
	forDuration, _ := strconv.Atoi(c.FormValue("for_duration"))

	override := c.FormValue("override") == "true"

	// Update alert
	var err error
	var affected int64 = 1
	if id > 0 {
		err = db.UpsertServerAlert(tid, id, item, threshold, forDuration)
	} else {
		affected, err = db.UpsertTeamAlert(tid, item, threshold, forDuration, override)
	}
	if err != nil {
		if errors.Is(err, db.ErrAlertNotFound) {
			return c.JSON(400, _type.H{
				Code: "invalid",
				Msg:  "Alert item not found",
			})
		}
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Alert updated successfully",
		Data: affected,
	})
}
