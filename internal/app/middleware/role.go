package middleware

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func TeamAccess(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		uid, _ := c.Get("uid").(int64)
		tid, _ := c.Get("tid").(int64)

		role, err := db.GetTeamRole(c.Request().Context(), uid, tid)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				if clearErr := clearRevokedTeamContext(c, uid, tid); clearErr != nil {
					return utils.ErrorHandler(c, clearErr, "Failed to clear revoked team access")
				}
				return c.JSON(403, _type.H{
					Code: "team_access_revoked",
					Msg:  "Team access has been revoked",
				})
			}
			return utils.ErrorHandler(c, err, "Database error")
		}
		c.Set("role", role)

		return next(c)
	}
}

// UserRole is retained for compatibility with packages that construct their
// own route groups. Team-scoped application routes use TeamAccess explicitly.
func UserRole(next echo.HandlerFunc) echo.HandlerFunc {
	return TeamAccess(next)
}

func clearRevokedTeamContext(c *echo.Context, uid, tid int64) error {
	if tid > 0 {
		if err := db.ClearUserActiveTeam(uid, tid); err != nil {
			return err
		}
	}
	sess, err := session.Get("session", c)
	if err != nil {
		return err
	}
	sess.Values["tid"] = int64(0)
	c.Set("tid", int64(0))
	return sess.Save(c.Request(), c.Response())
}

func WriteAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		role, ok := c.Get("role").(int)
		if !ok || role != 0 {
			return c.JSON(403, _type.H{
				Code: "forbidden",
				Msg:  "Permission denied",
			})
		}
		return next(c)
	}
}

func TerminalAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		role, ok := c.Get("role").(int)
		if !ok || (role != 0 && role != 1) {
			return c.JSON(403, _type.H{
				Code: "forbidden",
				Msg:  "Permission denied",
			})
		}
		return next(c)
	}
}
