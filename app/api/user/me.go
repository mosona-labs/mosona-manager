package auser

import (
	"github.com/labstack/echo/v4"
	"mosona-manager/_type"
	"mosona-manager/db"
)

func me(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	tid, _ := c.Get("tid").(int64)

	userInfo, err := db.GetUserById(uid)
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
				"user": userInfo,
				"team": nil,
			},
		})
	} else {
		teamInfo, err := db.GetTeamById(tid)
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
				"user": userInfo,
				"team": teamInfo,
			},
		})
	}
}
