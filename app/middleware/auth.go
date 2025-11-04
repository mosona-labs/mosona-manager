package middleware

import (
	"mosona-manager/_type"
	"mosona-manager/db"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

func UserAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, err := session.Get("session", c)
		if err != nil {
			return c.JSON(500, _type.H{Code: "err", Msg: "Session error"})
		}

		// User ID
		uid := sess.Values["uid"]
		if uid == nil || uid == 0 {
			return c.JSON(400, _type.H{Code: "login", Msg: "permission denied"})
		}
		userAgent := sess.Values["user_agent"]
		if userAgent == nil || userAgent != c.Request().Header.Get("User-Agent") {
			return c.JSON(400, _type.H{Code: "login", Msg: "permission denied"})
		}

		// Team ID
		tid, _ := sess.Values["tid"]
		if tid == nil || tid == 0 {
			activeTid, err := db.GetUserActiveTeam(uid.(int64))
			if err != nil {
				return c.JSON(500, _type.H{Code: "err", Msg: "Database error"})
			}
			sess.Values["tid"] = activeTid
			if err = sess.Save(c.Request(), c.Response()); err != nil {
				return c.JSON(500, _type.H{Code: "error", Msg: "Session update failed"})
			}
		}

		c.Set("uid", uid)
		c.Set("tid", tid)

		return next(c)
	}
}

func AdminAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, err := session.Get("session", c)
		if err != nil {
			return c.JSON(500, _type.H{Code: "err", Msg: "Session error"})
		}

		uid := sess.Values["uid"]
		if uid == nil || uid == 0 {
			return c.JSON(400, _type.H{Code: "login", Msg: "permission denied"})
		}
		userAgent := sess.Values["user_agent"]
		if userAgent == nil || userAgent != c.Request().Header.Get("User-Agent") {
			return c.JSON(400, _type.H{Code: "login", Msg: "permission denied"})
		}

		userInfo, err := db.GetUserById(uid.(int64))
		if err != nil || userInfo.ID == 0 {
			return c.JSON(400, _type.H{Code: "login", Msg: "permission denied"})
		}

		if !userInfo.IsAdmin {
			return c.JSON(400, _type.H{Code: "no_admin", Msg: "permission denied"})
		}

		c.Set("uid", userInfo.ID)

		return next(c)
	}
}
