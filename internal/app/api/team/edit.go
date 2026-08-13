package ateam

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/redis"
	"mosona-manager/internal/utils"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

var errInvalidTeamMembers = errors.New("invalid team members")
var errTeamEditForbidden = errors.New("team edit actor is no longer an administrator")

const teamAvatarDirectory = "./avatars"

func edit(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	uid, _ := c.Get("uid").(int64)
	requestedTID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if requestedTID <= 0 || requestedTID != tid {
		return c.JSON(400, _type.H{Code: "error", Msg: "Invalid team ID"})
	}

	form, err := c.FormValues()
	if err != nil {
		if errors.Is(err, echo.ErrStatusRequestEntityTooLarge) {
			return echo.ErrStatusRequestEntityTooLarge
		}
		return c.JSON(400, _type.H{Code: "error", Msg: "Invalid form data"})
	}
	name := form.Get("name")
	description := form.Get("description")
	avatarColor := form.Get("avatar_color")

	var members = make([]_type.TeamUsersRole, 0)
	if err = json.Unmarshal([]byte(form.Get("members")), &members); err != nil {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid member data",
		})
	}
	if name == "" || avatarColor == "" {
		return c.JSON(400, _type.H{Code: "error", Msg: "Invalid input"})
	}

	var avatarURL *string
	var stagedAvatarPath string
	var publishedAvatarPath string
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
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Failed to open avatar image",
			})
		}

		avatarFileName, uuidErr := uuid.NewUUID()
		if uuidErr != nil {
			_ = file.Close()
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Failed to generate avatar filename",
			})
		}
		avatarName := avatarFileName.String() + ".avif"
		stagedAvatarPath = filepath.Join(teamAvatarDirectory, "."+avatarFileName.String()+".pending.avif")
		publishedAvatarPath = filepath.Join(teamAvatarDirectory, avatarName)
		if err = utils.ConvertAvatar(file, teamAvatarDirectory, "."+avatarFileName.String()+".pending"); err != nil {
			_ = file.Close()
			_ = os.Remove(stagedAvatarPath)
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Failed to process avatar image",
			})
		}
		_ = file.Close()
		avatarURL = &avatarName
	}

	avatarPublished := false
	commitAttempted := false
	defer func() {
		if stagedAvatarPath != "" {
			_ = os.Remove(stagedAvatarPath)
		}
		if avatarPublished && !commitAttempted {
			_ = os.Remove(publishedAvatarPath)
		}
	}()

	// Update team members
	tx, err := db.Db.Beginx()
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	defer func() { _ = tx.Rollback() }()

	removedUserIDs, oldAvatar, err := updateTeam(tx, tid, uid, name, description, avatarColor, avatarURL, members)
	if err != nil {
		if errors.Is(err, errInvalidTeamMembers) {
			return c.JSON(400, _type.H{Code: "error", Msg: "Invalid member data"})
		}
		if errors.Is(err, errTeamEditForbidden) {
			return c.JSON(403, _type.H{Code: "forbidden", Msg: "Team administrator access required"})
		}
		return utils.ErrorHandler(c, err, "Database error")
	}

	if avatarURL != nil {
		if err = os.Rename(stagedAvatarPath, publishedAvatarPath); err != nil {
			return utils.ErrorHandler(c, err, "Failed to publish avatar image")
		}
		stagedAvatarPath = ""
		avatarPublished = true
	}

	commitAttempted = true
	// Commit errors can have an unknown outcome; retaining a published file avoids
	// breaking a database update that PostgreSQL may already have committed.
	if err = tx.Commit(); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	if avatarURL != nil {
		if err = removeOldTeamAvatar(oldAvatar, *avatarURL); err != nil {
			log.Printf("remove old avatar for team %d: %v", tid, err)
		}
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

func updateTeam(
	tx *sqlx.Tx,
	teamID, actorID int64,
	name, description, avatarColor string,
	newAvatar *string,
	members []_type.TeamUsersRole,
) ([]int64, string, error) {
	var ownerID int64
	var oldAvatar string
	if err := tx.QueryRow(
		"SELECT owner_id, image FROM teams WHERE id = $1 FOR UPDATE",
		teamID,
	).Scan(&ownerID, &oldAvatar); err != nil {
		return nil, "", err
	}

	var actorRole int
	if err := tx.QueryRow(
		"SELECT role FROM m_team_user WHERE team_id = $1 AND user_id = $2 FOR UPDATE",
		teamID, actorID,
	).Scan(&actorRole); err != nil {
		return nil, "", err
	}
	if actorRole != 0 {
		return nil, "", errTeamEditForbidden
	}
	if err := validateTeamMembers(ownerID, actorID, members); err != nil {
		return nil, "", err
	}

	rows, err := tx.Query(
		"SELECT user_id FROM m_team_user WHERE team_id = $1 ORDER BY user_id FOR UPDATE",
		teamID,
	)
	if err != nil {
		return nil, "", err
	}
	existing := make(map[int64]struct{})
	for rows.Next() {
		var userID int64
		if err = rows.Scan(&userID); err != nil {
			_ = rows.Close()
			return nil, "", err
		}
		existing[userID] = struct{}{}
	}
	if err = rows.Close(); err != nil {
		return nil, "", err
	}
	if err = rows.Err(); err != nil {
		return nil, "", err
	}

	requested := make(map[int64]struct{}, len(members))
	for _, member := range members {
		requested[member.ID] = struct{}{}
		if _, err = tx.Exec(`
			INSERT INTO m_team_user (team_id, user_id, role) VALUES ($1, $2, $3)
			ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role
		`, teamID, member.ID, member.Role); err != nil {
			return nil, "", err
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
			return nil, "", err
		}
		if _, err = tx.Exec(
			"DELETE FROM m_team_user WHERE team_id = $1 AND user_id = $2",
			teamID, userID,
		); err != nil {
			return nil, "", err
		}
		removed = append(removed, userID)
	}

	avatar := oldAvatar
	if newAvatar != nil {
		avatar = *newAvatar
	}
	if _, err = tx.Exec(
		"UPDATE teams SET name = $1, description = $2, color = $3, image = $4 WHERE id = $5",
		name, description, avatarColor, avatar, teamID,
	); err != nil {
		return nil, "", err
	}
	return removed, oldAvatar, nil
}

func validateTeamMembers(ownerID, actorID int64, members []_type.TeamUsersRole) error {
	seen := make(map[int64]struct{}, len(members))
	hasAdministrator := false
	ownerIsAdministrator := false
	actorIsAdministrator := false
	for _, member := range members {
		if member.ID <= 0 || member.Role < 0 || member.Role > 2 {
			return errInvalidTeamMembers
		}
		if _, exists := seen[member.ID]; exists {
			return errInvalidTeamMembers
		}
		seen[member.ID] = struct{}{}
		if member.Role == 0 {
			hasAdministrator = true
			ownerIsAdministrator = ownerIsAdministrator || member.ID == ownerID
			actorIsAdministrator = actorIsAdministrator || member.ID == actorID
		}
	}
	if !hasAdministrator || !ownerIsAdministrator || !actorIsAdministrator {
		return errInvalidTeamMembers
	}
	return nil
}

func removeOldTeamAvatar(oldAvatar, newAvatar string) error {
	return removeTeamAvatarFile(teamAvatarDirectory, oldAvatar, newAvatar)
}

func removeTeamAvatarFile(directory, oldAvatar, newAvatar string) error {
	if oldAvatar == "" || oldAvatar == newAvatar {
		return nil
	}
	if filepath.Base(oldAvatar) != oldAvatar {
		return fmt.Errorf("invalid old avatar filename %q", oldAvatar)
	}
	if err := os.Remove(filepath.Join(directory, oldAvatar)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
