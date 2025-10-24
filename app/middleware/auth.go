package middleware

import (
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"mosona-manager/_type"
	"mosona-manager/db"
)

func UserAuth(next echo.HandlerFunc) echo.HandlerFunc {
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

		c.Set("uid", uid)

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
