package ateam

import (
	"mosona-manager/_type"
	"mosona-manager/db"
	"strconv"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

func leave(c echo.Context) error {
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
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	// If the user is the owner: transfer ownership
	if isOwner {
		members, err := db.GetTeamMembers(targetId)
		if err != nil {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Database error",
			})
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
				return c.JSON(500, _type.H{
					Code: "error",
					Msg:  "Database error",
				})
			}
		} else {
			if err = db.RemoveTeam(targetId); err != nil {
				return c.JSON(500, _type.H{
					Code: "error",
					Msg:  "Database error",
				})
			}
		}
	}

	// Remove user from team
	if err = db.RemoveUserFromTeam(uid, targetId); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	// Current active team check
	if targetId == tid {
		teams, err := db.GetTeamsByUserId(uid)
		if err != nil {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Database error",
			})
		}

		var newActiveTeamId int64 = 0
		if len(teams) > 0 {
			newActiveTeamId = teams[0].ID
		}

		if err = db.SetUserActiveTeam(uid, newActiveTeamId); err != nil {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Database error",
			})
		}
		sess, err := session.Get("session", c)
		if err != nil {
			return c.JSON(500, _type.H{Code: "error", Msg: "Session error"})
		}
		sess.Values["tid"] = newActiveTeamId
		if err = sess.Save(c.Request(), c.Response()); err != nil {
			return c.JSON(500, _type.H{Code: "error", Msg: "Session update failed"})
		}
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Left team successfully",
	})
}
