package ateam

import (
	"encoding/json"
	"fmt"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/redis"
	"mosona-manager/internal/utils"
	"os"
	"path"
	"strconv"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

func edit(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	uid, _ := c.Get("uid").(int64)
	requestedTID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if requestedTID <= 0 || requestedTID != tid {
		return c.JSON(400, _type.H{Code: "error", Msg: "Invalid team ID"})
	}

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
	tx, err := db.Db.Beginx()
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	defer func() { _ = tx.Rollback() }()

	removedUserIDs, err := updateTeamMembers(tx, tid, uid, members)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
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
		return utils.ErrorHandler(c, err, "Database error")
	}

	if err = tx.Commit(); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	for _, removedUserID := range removedUserIDs {
		if err = redis.RemoveUserTeamSessions(c.Request().Context(), removedUserID, tid); err != nil {
			log.Printf("revoke removed member %d sessions for team %d: %v", removedUserID, tid, err)
		}
	}

	// Log action
	influx.LogAdd(
		tid, uid, "team", "Edit Team: "+name+" (ID"+strconv.FormatInt(tid, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Team updated",
	})
}

func updateTeamMembers(tx *sqlx.Tx, teamID, actorID int64, members []_type.TeamUsersRole) ([]int64, error) {
	var lockedTeamID int64
	if err := tx.QueryRow("SELECT id FROM teams WHERE id = $1 FOR UPDATE", teamID).Scan(&lockedTeamID); err != nil {
		return nil, err
	}

	var actorRole int
	if err := tx.QueryRow(
		"SELECT role FROM m_team_user WHERE team_id = $1 AND user_id = $2 FOR UPDATE",
		teamID, actorID,
	).Scan(&actorRole); err != nil {
		return nil, err
	}
	if actorRole != 0 {
		return nil, fmt.Errorf("team edit actor is no longer an administrator")
	}

	rows, err := tx.Query(
		"SELECT user_id FROM m_team_user WHERE team_id = $1 ORDER BY user_id FOR UPDATE",
		teamID,
	)
	if err != nil {
		return nil, err
	}
	existing := make(map[int64]struct{})
	for rows.Next() {
		var userID int64
		if err = rows.Scan(&userID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		existing[userID] = struct{}{}
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	requested := make(map[int64]struct{}, len(members))
	for _, member := range members {
		requested[member.ID] = struct{}{}
		if _, err = tx.Exec(`
			INSERT INTO m_team_user (team_id, user_id, role) VALUES ($1, $2, $3)
			ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role
		`, teamID, member.ID, member.Role); err != nil {
			return nil, err
		}
	}

	removed := make([]int64, 0)
	for userID := range existing {
		if _, keep := requested[userID]; keep {
			continue
		}
		if _, err = tx.Exec(
			"DELETE FROM users_config WHERE uid = $1 AND active_team = $2",
			userID, teamID,
		); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(
			"DELETE FROM m_team_user WHERE team_id = $1 AND user_id = $2",
			teamID, userID,
		); err != nil {
			return nil, err
		}
		removed = append(removed, userID)
	}
	return removed, nil
}
