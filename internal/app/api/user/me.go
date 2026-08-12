package auser

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func me(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	tid, _ := c.Get("tid").(int64)

	userInfo, err := db.GetUserById(uid)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	if tid == 0 {
		tid, err = db.GetUserActiveTeam(uid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return utils.ErrorHandler(c, err, "Database error")
		}
		if tid != 0 {
			c.Set("tid", tid)
		}
	}
	if tid != 0 {
		if _, roleErr := db.GetTeamRole(c.Request().Context(), uid, tid); roleErr != nil {
			if !errors.Is(roleErr, sql.ErrNoRows) {
				return utils.ErrorHandler(c, roleErr, "Database error")
			}
			if err = db.ClearUserActiveTeam(uid, tid); err != nil {
				return utils.ErrorHandler(c, err, "Database error")
			}
			sess, sessionErr := session.Get("session", c)
			if sessionErr != nil {
				return utils.ErrorHandler(c, sessionErr, "Session error")
			}
			sess.Values["tid"] = int64(0)
			if sessionErr = sess.Save(c.Request(), c.Response()); sessionErr != nil {
				return utils.ErrorHandler(c, sessionErr, "Session update failed")
			}
			tid = 0
			c.Set("tid", tid)
		}
	}

	teams, err := db.GetTeamsByUserId(uid)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
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
		team, err := db.GetTeamById(tid)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				sess, err := session.Get("session", c)
				if err != nil {
					return utils.ErrorHandler(c, err, "Session error")
				}
				sess.Values["tid"] = 0
				if err = sess.Save(c.Request(), c.Response()); err != nil {
					return utils.ErrorHandler(c, err, "Session update failed")
				}
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
				return utils.ErrorHandler(c, err, "Database error")
			}
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
