package muser

import (
	"database/sql"
	"errors"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
)

func del(c *echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid user ID",
		})
	}

	deletedUsername, err := db.DeleteUser(c.Request().Context(), id, c.QueryParam("confirm"))
	if err != nil {
		var ownsTeams *db.UserOwnsTeamsError
		if errors.As(err, &ownsTeams) {
			return c.JSON(http.StatusConflict, _type.H{
				Code: "user_owns_teams",
				Msg:  "Transfer or delete owned teams before deleting this user",
				Data: _type.Map{"teams": ownsTeams.Teams},
			})
		}
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, _type.H{
				Code: "user_not_found",
				Msg:  "User not found",
			})
		}
		if errors.Is(err, db.ErrDeleteUserConfirmationMismatch) {
			return c.JSON(http.StatusBadRequest, _type.H{
				Code: "delete_confirmation_mismatch",
				Msg:  "Confirmation username does not match",
			})
		}
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Log action
	influx.LogAdd(
		0, c.Get("uid").(int64), "user",
		fmt.Sprintf("delete user %q (ID: %d)", deletedUsername, id),
		c.RealIP(), c.Request().UserAgent(), "high",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "User deleted successfully",
	})
}
