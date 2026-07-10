package middleware

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func UserAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if !config.DynamicConf.Init {
			return c.JSON(200, _type.H{
				Code: "init_required",
				Msg:  "System initialization required",
			})
		}

		// Session
		sess, err := session.Get("session", c)
		if err != nil {
			return utils.ErrorHandler(c, err, "Session error")
		}

		// 2FA Required
		if sess.Values["pre_2fa_uid"] != nil {
			return utils.ErrorHandler(c, err, "Two-factor authentication required")
		}

		// User ID
		uid := sess.Values["uid"]
		if uid == nil || uid == 0 {
			return c.JSON(401, _type.H{
				Code: "login",
				Msg:  "Login error",
			})
		}
		if !SessionBindingOK(c, sess) {
			return c.JSON(400, _type.H{Code: "login", Msg: "permission denied"})
		}

		// Team ID
		tid, _ := sess.Values["tid"]
		if tid == nil || tid == 0 {
			activeTid, err := db.GetUserActiveTeam(uid.(int64))
			if err != nil {
				return utils.ErrorHandler(c, err, "Database error")
			}
			sess.Values["tid"] = activeTid
			tid = activeTid
			if err = sess.Save(c.Request(), c.Response()); err != nil {
				return utils.ErrorHandler(c, err, "Session update failed")
			}
		}

		c.Set("uid", uid)
		c.Set("tid", tid)
		c.Set("sid", sess.ID)

		return next(c)
	}
}

func AdminAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if !config.DynamicConf.Init {
			return c.JSON(200, _type.H{
				Code: "init_required",
				Msg:  "System initialization required",
			})
		}

		// Session
		sess, err := session.Get("session", c)
		if err != nil {
			return c.JSON(500, _type.H{Code: "error", Msg: "Session error"})
		}

		// 2FA Required
		if sess.Values["pre_2fa_uid"] != nil {
			return c.JSON(200, _type.H{Code: "2fa_required", Msg: "Two-factor authentication required"})
		}

		uid := sess.Values["uid"]
		if uid == nil || uid == 0 {
			return c.JSON(400, _type.H{Code: "login", Msg: "permission denied"})
		}
		if !SessionBindingOK(c, sess) {
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

func InitAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if config.DynamicConf.Init {
			return c.JSON(400, _type.H{
				Code: "already_initialized",
				Msg:  "System already initialized",
			})
		}
		return next(c)
	}
}
