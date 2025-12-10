package ateam

import (
	"encoding/json"
	db2 "mosona-manager/internal/db"
	_type2 "mosona-manager/pkg/_type"
	"mosona-manager/pkg/utils"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func create(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)

	name := c.FormValue("name")
	description := c.FormValue("description")
	avatarColor := c.FormValue("avatar_color")

	// Parse member IDs
	var members = make([]_type2.TeamUsersRole, 0)
	if err := json.Unmarshal([]byte(c.FormValue("members")), &members); err != nil {
		return c.JSON(400, _type2.H{
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
		return c.JSON(400, _type2.H{
			Code: "error",
			Msg:  "You must be the owner of the team",
		})
	}

	// Validate input
	if name == "" || avatarColor == "" {
		return c.JSON(400, _type2.H{
			Code: "error",
			Msg:  "Invalid input",
		})
	}

	// Get avatar image
	var avatarUrl = ""
	avatarImage, err := c.FormFile("avatar_image")
	if err == nil && avatarImage != nil {
		allowedTypes := []string{"image/jpeg", "image/png", "image/gif", "image/webp"}
		contentType := avatarImage.Header.Get("Content-Type")
		isImage := false
		for _, t := range allowedTypes {
			if contentType == t {
				isImage = true
				break
			}
		}
		if !isImage {
			return c.JSON(400, _type2.H{
				Code: "error",
				Msg:  "Invalid file type, only images are allowed",
			})
		}
		file, err := avatarImage.Open()
		if err != nil {
			return c.JSON(500, _type2.H{
				Code: "error",
				Msg:  "Failed to open avatar image",
			})
		}
		defer func() {
			_ = file.Close()
		}()

		avatarFileName, err := uuid.NewUUID()
		if err != nil {
			return c.JSON(500, _type2.H{
				Code: "error",
				Msg:  "Failed to generate avatar filename",
			})
		}
		if err = utils.ConvertAvatar(file, "./avatars", avatarFileName.String()); err != nil {
			return c.JSON(500, _type2.H{
				Code: "error",
				Msg:  "Failed to process avatar image",
			})
		}
		avatarUrl = avatarFileName.String() + ".avif"
	}

	teamId, err := db2.CreateTeam(
		name,
		description,
		avatarColor,
		avatarUrl,
		members,
		uid,
	)
	if err != nil {
		return c.JSON(500, _type2.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	// Set Active Team
	_ = db2.SetUserActiveTeam(uid, teamId)

	return c.JSON(200, _type2.H{
		Code: "ok",
		Msg:  "Team created successfully",
		Data: teamId,
	})
}
