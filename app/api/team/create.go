package ateam

import (
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"mosona-manager/_type"
	"mosona-manager/db"
	"mosona-manager/utils"
	"strconv"
	"strings"
)

func create(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)

	name := c.FormValue("name")
	description := c.FormValue("description")
	avatarColor := c.FormValue("avatar_color")

	// Get plan ID
	planId, _ := strconv.ParseInt(c.FormValue("plan_id"), 10, 64)

	// Parse member IDs
	members := c.FormValue("members")
	memberIds := make([]int64, 0, len(strings.Split(members, ",")))
	hasOwner := false
	for _, memberUID := range strings.Split(members, ",") {
		memberUID = strings.TrimSpace(memberUID)
		if memberUID == "" {
			continue
		}
		uidInt64, err := strconv.ParseInt(memberUID, 10, 64)
		if err != nil || uidInt64 == 0 {
			continue
		}
		memberIds = append(memberIds, uidInt64)
		if uidInt64 == uid {
			hasOwner = true
		}
	}
	if !hasOwner {
		memberIds = append(memberIds, uid)
	}

	// Validate input
	if name == "" || avatarColor == "" || len(memberIds) == 0 || planId == 0 {
		return c.JSON(400, _type.H{
			Code: "err",
			Msg:  "Invalid input",
		})
	}

	// Plan
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
		memberIds,
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
