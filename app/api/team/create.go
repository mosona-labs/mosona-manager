package ateam

import (
	"database/sql"
	"encoding/json"
	"errors"
	"mosona-manager/_type"
	"mosona-manager/db"
	"mosona-manager/utils"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func create(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)

	name := c.FormValue("name")
	description := c.FormValue("description")
	avatarColor := c.FormValue("avatar_color")

	// Get plan ID
	planId, _ := strconv.ParseInt(c.FormValue("plan_id"), 10, 64)
	planInfo, err := db.GetPlanById(planId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(400, _type.H{
				Code: "err",
				Msg:  "Plan not found",
			})
		} else {
			return c.JSON(500, _type.H{
				Code: "err",
				Msg:  "Database error",
			})
		}
	}

	// Parse member IDs
	var members = make([]_type.TeamUsersRole, 0)
	if err = json.Unmarshal([]byte(c.FormValue("members")), &members); err != nil {
		return c.JSON(400, _type.H{
			Code: "err",
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
			Code: "err",
			Msg:  "You must be the owner of the team",
		})
	}
	if len(members) > planInfo.MaxMember && planInfo.MaxMember != -1 {
		return c.JSON(400, _type.H{
			Code: "err",
			Msg:  "Member count exceeds plan limit",
		})
	}

	// Validate input
	if name == "" || avatarColor == "" || planId == 0 {
		return c.JSON(400, _type.H{
			Code: "err",
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
			return c.JSON(400, _type.H{
				Code: "err",
				Msg:  "Invalid file type, only images are allowed",
			})
		}
		file, err := avatarImage.Open()
		if err != nil {
			return c.JSON(500, _type.H{
				Code: "err",
				Msg:  "Failed to open avatar image",
			})
		}
		defer func() {
			_ = file.Close()
		}()

		avatarFileName, err := uuid.NewUUID()
		if err != nil {
			return c.JSON(500, _type.H{
				Code: "err",
				Msg:  "Failed to generate avatar filename",
			})
		}
		if err = utils.ConvertAvatar(file, "./avatars", avatarFileName.String()); err != nil {
			return c.JSON(500, _type.H{
				Code: "err",
				Msg:  "Failed to process avatar image",
			})
		}
		avatarUrl = avatarFileName.String() + ".avif"
	}

	teamId, err := db.CreateTeam(
		name,
		description,
		avatarColor,
		avatarUrl,
		members,
		planInfo.MaxServer,
		planInfo.MaxAlert,
		planInfo.MaxMember,
		planId,
		uid,
	)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error",
		})
	}

	// Set Active Team
	_ = db.SetUserActiveTeam(uid, teamId)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Team created successfully",
		Data: teamId,
	})
}
