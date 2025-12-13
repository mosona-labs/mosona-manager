package auser

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/redis"

	"github.com/labstack/echo/v4"
)

func sessions(c echo.Context) error {
	data, err := redis.GetUserSessions(c.Request().Context(), c.Get("uid").(int64))
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
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
			Code: "error",
			Msg:  "Database error",
		})
	} else if !yes {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Session not found",
		})
	}

	if err := redis.RemoveSessionIDs(ctx, []string{sid}); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
	})
}

func sessionRevokeAll(c echo.Context) error {
	uid := c.Get("uid").(int64)

	ctx := c.Request().Context()

	sids, err := redis.GetUserSessions(ctx, uid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	var sidList []string
	for _, s := range sids {
		sidList = append(sidList, s.ID)
	}

	if err = redis.RemoveSessionIDs(ctx, sidList); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
	})
}
