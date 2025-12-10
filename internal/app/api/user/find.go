package auser

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/db"
	"mosona-manager/pkg/_type"

	"github.com/labstack/echo/v4"
)

func find(c echo.Context) error {
	email := c.FormValue("email")

	user, err := db.GetUserByEmail(email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(200, _type.H{
				Code: "ok",
				Msg:  "User not found",
				Data: nil,
			})
		} else {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Database error",
			})
		}
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: user,
	})
}
