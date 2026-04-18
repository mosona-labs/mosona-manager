package auser

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func setActiveTeam(c *echo.Context) error {
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
		return utils.ErrorHandler(c, err, "Failed to set active team")
	}

	sess, err := session.Get("session", c)
	if err != nil {
		return utils.ErrorHandler(c, err, "Session error")
	}
	sess.Values["tid"] = teamID
	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return utils.ErrorHandler(c, err, "Session update failed")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Active team updated",
	})
}
