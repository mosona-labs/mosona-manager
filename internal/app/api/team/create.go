package ateam

import (
	"encoding/json"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/google/uuid"
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
	for _, m := range members {
		if m.ID == uid {
			hasOwner = true
		}
	}
	if !hasOwner {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "You must be the owner of the team",
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
	_ = db.SetUserActiveTeam(uid, teamId)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Team created successfully",
		Data: teamId,
	})
}
