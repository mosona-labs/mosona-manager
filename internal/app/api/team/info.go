package ateam

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"

	"github.com/labstack/echo/v4"
)

func info(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	if tid == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid team data",
		})
	}

	team, err := db.GetTeamById(tid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	members, err := db.GetTeamMembers(tid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"team":    team,
			"members": members,
		},
	})
}
