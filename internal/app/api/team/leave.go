package ateam

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

func leave(c *echo.Context) error {
	uid := c.Get("uid").(int64)
	tid := c.Get("tid").(int64)

	targetId, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if targetId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid team ID",
		})
	}

	isOwner, err := db.IsTeamOwner(targetId, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			teams, teamErr := db.GetTeamsByUserId(uid)
			if teamErr != nil {
				return utils.ErrorHandler(c, teamErr, "Database error")
			}
			if len(teams) == 0 {
				if err = saveActiveTeam(c, uid, 0); err != nil {
					return err
				}
				return c.JSON(200, _type.H{
					Code: "ok",
					Msg:  "Left team successfully",
				})
			}
		}
		return utils.ErrorHandler(c, err, "Database error")
	}

	// If the user is the owner: transfer ownership
	if isOwner {
		members, err := db.GetTeamMembers(targetId)
		if err != nil {
			return utils.ErrorHandler(c, err, "Database error")
		}
		// Select a new owner
		var newOwnerId int64 = -1
		for _, m := range members {
			if m.ID != uid {
				newOwnerId = m.ID
				break
			}
		}
		// Transfer: must have a new owner
		if newOwnerId != -1 {
			if err = db.TransferTeamOwnership(targetId, newOwnerId); err != nil {
				return utils.ErrorHandler(c, err, "Database error")
			}
		} else {
			if err = db.RemoveTeam(targetId); err != nil {
				return utils.ErrorHandler(c, err, "Database error")
			}
		}
	}

	// Remove user from team
	if err = db.RemoveUserFromTeam(uid, targetId); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Current active team check
	if targetId == tid {
		teams, err := db.GetTeamsByUserId(uid)
		if err != nil {
			return utils.ErrorHandler(c, err, "Database error")
		}

		var newActiveTeamId int64 = 0
		if len(teams) > 0 {
			newActiveTeamId = teams[0].ID
		}

		if err = saveActiveTeam(c, uid, newActiveTeamId); err != nil {
			return err
		}
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Left team successfully",
	})
}

func saveActiveTeam(c *echo.Context, uid, tid int64) error {
	if err := db.SetUserActiveTeam(uid, tid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			tid = 0
			if err = db.SetUserActiveTeam(uid, tid); err != nil {
				return utils.ErrorHandler(c, err, "Database error")
			}
		} else {
			return utils.ErrorHandler(c, err, "Database error")
		}
	}

	sess, err := session.Get("session", c)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session error"})
	}
	sess.Values["tid"] = tid
	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session update failed"})
	}
	return nil
}
