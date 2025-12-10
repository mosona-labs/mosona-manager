package auser

import (
	"database/sql"
	"errors"
	db2 "mosona-manager/internal/db"
	"mosona-manager/pkg/_type"

	"github.com/labstack/echo/v4"
)

func me(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	tid, _ := c.Get("tid").(int64)

	userInfo, err := db2.GetUserById(uid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	if tid == 0 {
		tid, err = db2.GetUserActiveTeam(uid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Database error",
			})
		}
		if tid != 0 {
			c.Set("tid", tid)
		}
	}

	teams, err := db2.GetTeamsByUserId(uid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	if tid == 0 {
		return c.JSON(200, _type.H{
			Code: "ok",
			Msg:  "Success",
			Data: map[string]interface{}{
				"user":  userInfo,
				"team":  nil,
				"teams": teams,
			},
		})
	} else {
		team, err := db2.GetTeamById(tid)
		if err != nil {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Database error",
			})
		}

		return c.JSON(200, _type.H{
			Code: "ok",
			Msg:  "Success",
			Data: map[string]interface{}{
				"user":  userInfo,
				"team":  team,
				"teams": teams,
			},
		})
	}
}
