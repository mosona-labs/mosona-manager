package middleware

import (
	"mosona-manager/internal/db"
	"mosona-manager/pkg/_type"

	"github.com/labstack/echo/v4"
)

func UserRole(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		uid, _ := c.Get("uid").(int64)
		tid, _ := c.Get("tid").(int64)

		var role int
		if err := db.Db.QueryRow(
			"SELECT role FROM m_team_user WHERE user_id = $1 AND team_id = $2",
			uid, tid,
		).Scan(&role); err != nil {
			c.Set("role", 2)
		} else {
			c.Set("role", role)
		}

		return next(c)
	}
}

func WriteAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		role, _ := c.Get("role").(int)
		if role != 0 {
			return c.JSON(403, _type.H{
				Code: "forbidden",
				Msg:  "Permission denied",
			})
		}
		return next(c)
	}
}

func TerminalAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		role, _ := c.Get("role").(int)
		if role == 2 {
			return c.JSON(403, _type.H{
				Code: "forbidden",
				Msg:  "Permission denied",
			})
		}
		return next(c)
	}
}
