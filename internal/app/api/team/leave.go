package ateam

import (
	"database/sql"
	"errors"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/redis"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func leave(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	activeTeamID, _ := c.Get("tid").(int64)
	targetTeamID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if targetTeamID <= 0 {
		return c.JSON(400, _type.H{Code: "error", Msg: "Invalid team ID"})
	}

	ctx := c.Request().Context()
	if err := db.LeaveTeam(ctx, uid, targetTeamID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(403, _type.H{Code: "forbidden", Msg: "Not a member of this team"})
		}
		return utils.ErrorHandler(c, err, "Database error")
	}

	if targetTeamID == activeTeamID {
		sess, err := session.Get("session", c)
		if err != nil {
			_ = redis.RemoveUserTeamSessions(ctx, uid, targetTeamID)
			return utils.ErrorHandler(c, err, "Session error")
		}
		sess.Values["tid"] = int64(0)
		c.Set("tid", int64(0))
		if err = sess.Save(c.Request(), c.Response()); err != nil {
			_ = redis.RemoveUserTeamSessions(ctx, uid, targetTeamID)
			return utils.ErrorHandler(c, err, "Session update failed")
		}
	}
	if err := redis.RemoveUserTeamSessions(ctx, uid, targetTeamID); err != nil {
		log.Printf("revoke user %d sessions after leaving team %d: %v", uid, targetTeamID, err)
	}

	return c.JSON(200, _type.H{Code: "ok", Msg: "Left team successfully"})
}
