package auth

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"time"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func loginSession(c *echo.Context, uid int64, isAdmin bool) *_type.H {
	// Session
	sess, err := session.Get("session", c)
	if err != nil {
		return &_type.H{Code: "error", Msg: "Session init error"}
	}

	// Already logged in
	if sess.Values["uid"] != nil {
		return &_type.H{Code: "error", Msg: "Already logged in"}
	}

	// Active Team
	activeTid, err := db.GetUserActiveTeam(uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return &_type.H{Code: "error", Msg: "Database error"}
	}
	sess.Values["uid"] = uid
	sess.Values["tid"] = activeTid
	sess.Values["user_agent"] = c.Request().Header.Get("User-Agent")
	sess.Values["time"] = time.Now().Unix()
	delete(sess.Values, "pre_2fa_uid")
	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return &_type.H{Code: "error", Msg: "Session save failed"}
	}

	loginEvent(uid, sess.ID, c.RealIP(), c.Request().Header.Get("User-Agent"), isAdmin)

	return nil
}
