package auser

import (
	"database/sql"
	"errors"
	"github.com/labstack/echo/v4"
	"mosona-manager/_type"
	"mosona-manager/db"
	"strconv"
)

// TODO: Frequent DB writes, high concurrency optimization require cache layer optimization
func setActiveTeam(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	teamID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if teamID == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid team ID",
		})
	}

	if err := db.SetUserActiveTeam(uid, teamID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Team not found or user is not a member",
			})
		}
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Active team updated",
	})
}
