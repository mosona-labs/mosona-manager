package muser

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/security/passwordhash"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v5"
)

func edit(c *echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid user ID",
		})
	}
	form, err := adminMutationForm(c.Request())
	if err != nil {
		return adminMutationError(c, err)
	}
	username := form.Get("username")
	email := form.Get("email")
	password := form.Get("password")
	verified := form.Get("verified") == "true"
	isAdmin := form.Get("is_admin") == "true"
	actorID := c.Get("uid").(int64)

	if username == "" || email == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Username, email cannot be empty",
		})
	}

	if actorID == id && !isAdmin {
		return adminMutationError(c, db.ErrCannotModifySelf)
	}

	reauthenticatedHash := ""
	if currentPassword := form.Get("current_password"); currentPassword != "" {
		reauthenticatedHash, err = reauthenticate(c, actorID, currentPassword)
		if err != nil {
			return adminMutationError(c, err)
		}
	}

	update := db.AdminUserUpdate{
		Username: username,
		Email:    email,
		Verified: verified,
		IsAdmin:  isAdmin,
	}
	if password != "" {
		newPassword, hashErr := passwordhash.Hash(password)
		if hashErr != nil {
			return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
		}
		update.PasswordHash = newPassword
		update.PasswordSalt = utils.RandomString(32)
	}
	if err := db.UpdateAdminUser(c.Request().Context(), actorID, id, update, reauthenticatedHash); err != nil {
		return adminMutationError(c, err)
	}

	// Log action
	influx.LogAdd(
		0, actorID, "user", "edit user ID "+strconv.FormatInt(id, 10),
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "User updated successfully",
	})
}
