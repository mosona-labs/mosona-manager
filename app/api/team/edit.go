package ateam

import (
	"encoding/json"
	"mosona-manager/_type"
	"mosona-manager/db"
	"mosona-manager/influx"
	"mosona-manager/utils"
	"os"
	"path"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func edit(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	uid, _ := c.Get("uid").(int64)

	name := c.FormValue("name")
	description := c.FormValue("description")
	avatarColor := c.FormValue("avatar_color")

	var members = make([]_type.TeamUsersRole, 0)
	if err := json.Unmarshal([]byte(c.FormValue("members")), &members); err != nil {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid member data",
		})
	}

	// Update team members
	tx, err := db.Db.Begin()
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	if _, err = tx.Exec("DELETE FROM m_team_user WHERE team_id = $1", tid); err != nil {
		_ = tx.Rollback()
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	for _, m := range members {
		if _, err = tx.Exec("INSERT INTO m_team_user (team_id, user_id, role) VALUES ($1, $2, $3)", tid, m.ID, m.Role); err != nil {
			_ = tx.Rollback()
			return c.JSON(500, _type.H{
				Code: "error",
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
				Code: "error",
				Msg:  "Invalid file type, only images are allowed",
			})
		}
		file, err := avatarImage.Open()
		if err != nil {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Failed to open avatar image",
			})
		}
		defer func() {
			_ = file.Close()
		}()

		avatarFileName, err := uuid.NewUUID()
		if err != nil {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Failed to generate avatar filename",
			})
		}
		if err = utils.ConvertAvatar(file, "./avatars", avatarFileName.String()); err != nil {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Failed to process avatar image",
			})
		}
		avatarUrl = avatarFileName.String() + ".avif"

		// Remove old avatar
		var oldAvatar string
		if err = tx.QueryRow("SELECT image FROM teams WHERE id = $1", tid).Scan(&oldAvatar); err == nil {
			if oldAvatar != "" {
				if err = os.Remove(path.Join("./avatars", oldAvatar)); err != nil && !os.IsNotExist(err) {
					return c.JSON(500, _type.H{
						Code: "error",
						Msg:  "Failed to remove old avatar image",
					})
				}
			}
		}
	}

	if _, err = tx.Exec(
		"UPDATE teams SET name = $1, description = $2, color = $3, image = $4 WHERE id = $5",
		name, description, avatarColor, avatarUrl, tid,
	); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	if err = tx.Commit(); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	// Log action
	influx.LogAdd(
		tid, uid, "team", "Edit Team: "+name+" (ID"+strconv.FormatInt(tid, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "high",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Team updated",
	})
}
