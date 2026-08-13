package ateam

import (
	"encoding/json"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func create(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)

	name := c.FormValue("name")
	description := c.FormValue("description")
	avatarColor := c.FormValue("avatar_color")

	// Parse member IDs
	var members = make([]_type.TeamUsersRole, 0)
	if err := json.Unmarshal([]byte(c.FormValue("members")), &members); err != nil {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid member data",
		})
	}
	hasOwner := false
	for i, m := range members {
		if m.ID == uid {
			hasOwner = true
			members[i].Role = 0
		}
	}
	if !hasOwner {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "You must be the owner of the team",
		})
	}
	if err := validateTeamMembers(uid, uid, members); err != nil {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid member data",
		})
	}

	// Validate input
	if name == "" || avatarColor == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid input",
		})
	}

	// Get avatar image
	var avatarUrl = ""
	avatarImage, err := c.FormFile("avatar_image")
	if err != nil && !errors.Is(err, http.ErrMissingFile) && !errors.Is(err, http.ErrNotMultipart) {
		return c.JSON(400, _type.H{Code: "error", Msg: "Invalid avatar upload"})
	}
	if err == nil && avatarImage != nil {
		if avatarImage.Size > utils.MaxAvatarBytes {
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Avatar image is too large",
			})
		}
		file, err := avatarImage.Open()
		if err != nil {
			return utils.ErrorHandler(c, err, "Failed to open avatar image")
		}
		defer func() {
			_ = file.Close()
		}()

		avatarFileName, err := uuid.NewUUID()
		if err != nil {
			return utils.ErrorHandler(c, err, "Failed to generate avatar filename")
		}
		if err = utils.ConvertAvatar(file, "./avatars", avatarFileName.String()); err != nil {
			return utils.ErrorHandler(c, err, "Failed to process avatar image")
		}
		avatarUrl = avatarFileName.String() + ".avif"
	}

	teamId, err := db.CreateTeam(
		name,
		description,
		avatarColor,
		avatarUrl,
		members,
		uid,
	)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Set Active Team
	if err = db.SetUserActiveTeam(uid, teamId); err != nil {
		return utils.ErrorHandler(c, err, "Failed to set active team")
	}
	sess, err := session.Get("session", c)
	if err != nil {
		return utils.ErrorHandler(c, err, "Session error")
	}
	sess.Values["tid"] = teamId
	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return utils.ErrorHandler(c, err, "Session update failed")
	}
	c.Set("tid", teamId)
	c.Set("role", 0)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Team created successfully",
		Data: teamId,
	})
}
