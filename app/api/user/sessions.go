package auser

import (
	"mosona-manager/_type"
	"mosona-manager/redis"

	"github.com/labstack/echo/v4"
)

func sessions(c echo.Context) error {
	data, err := redis.GetUserSessions(c.Request().Context(), c.Get("uid").(int64))
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error: " + err.Error(),
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"list":    data,
			"current": c.Get("sid").(string),
		},
	})
}

func sessionRevoke(c echo.Context) error {
	uid := c.Get("uid").(int64)
	sid := c.Param("sid")

	ctx := c.Request().Context()

	if yes, err := redis.CheckSessionOwnership(ctx, uid, sid); err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error: " + err.Error(),
		})
	} else if !yes {
		return c.JSON(400, _type.H{
			Code: "err",
			Msg:  "Session not found",
		})
	}

	if err := redis.RemoveSessionIDs(ctx, []string{sid}); err != nil {
		return c.JSON(500, _type.H{
			Code: "err",
			Msg:  "Database error: " + err.Error(),
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
	})
}
