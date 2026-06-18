package auth

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/db"
	"mosona-manager/internal/redis"
	"time"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func finalizeAuthenticatedSession(c *echo.Context, uid int64, maxAge int) (string, error) {
	sess, err := session.Get("session", c)
	if err != nil {
		return "", err
	}

	oldID := sess.ID
	ctx := c.Request().Context()

	if oldID != "" {
		if err = redis.RemoveSessionIDs(ctx, []string{oldID}); err != nil {
			return "", err
		}
	}

	sess.ID = ""

	activeTid, err := db.GetUserActiveTeam(uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	sess.Values["uid"] = uid
	sess.Values["tid"] = activeTid
	sess.Values["user_agent"] = c.Request().Header.Get("User-Agent")
	sess.Values["client_ip"] = c.RealIP()
	sess.Values["time"] = time.Now().Unix()
	delete(sess.Values, "pre_2fa_uid")

	sess.Options = sessionOptions(maxAge)
	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return "", err
	}

	if oldID != "" && oldID != sess.ID {
		_ = redis.RemoveUserSessionIDRefs(ctx, uid, []string{oldID})
	}

	return sess.ID, nil
}
