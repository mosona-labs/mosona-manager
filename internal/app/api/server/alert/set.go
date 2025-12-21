package aalert

import (
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
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

	// Update alert
	if err := db.UpsertServerAlert(tid, id, item, threshold, forDuration); err != nil {
		if errors.Is(err, db.ErrAlertNotFound) {
			return c.JSON(400, _type.H{
				Code: "invalid",
				Msg:  "Alert item not found",
			})
		}
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Alert updated successfully",
	})
}
