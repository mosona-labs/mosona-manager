package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mosona-manager/internal/db"
	"mosona-manager/internal/redis"
	"time"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

type authenticatedSessionDeps struct {
	getActiveTeam     func(int64) (int64, error)
	addSessionID      func(context.Context, int64, string) error
	removeSessionIDs  func(context.Context, []string) error
	removeSessionRefs func(context.Context, int64, []string) error
}

func finalizeAuthenticatedSession(c *echo.Context, uid int64, maxAge int) (string, error) {
	return finalizeAuthenticatedSessionWithDeps(c, uid, maxAge, authenticatedSessionDeps{
		getActiveTeam:     db.GetUserActiveTeam,
		addSessionID:      redis.AddSessionID,
		removeSessionIDs:  redis.RemoveSessionIDs,
		removeSessionRefs: redis.RemoveUserSessionIDRefs,
	})
}

func finalizeAuthenticatedSessionWithDeps(c *echo.Context, uid int64, maxAge int, deps authenticatedSessionDeps) (string, error) {
	sess, err := session.Get("session", c)
	if err != nil {
		return "", err
	}

	oldID := sess.ID
	ctx := c.Request().Context()

	if oldID != "" {
		if err = deps.removeSessionIDs(ctx, []string{oldID}); err != nil {
			return "", err
		}
	}

	sess.ID = ""

	activeTid, err := deps.getActiveTeam(uid)
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
	if sess.ID == "" {
		return "", errors.New("saved session has no ID")
	}

	if err = deps.addSessionID(ctx, uid, sess.ID); err != nil {
		removeSessionErr := deps.removeSessionIDs(ctx, []string{sess.ID})
		removeRefErr := deps.removeSessionRefs(ctx, uid, []string{sess.ID})
		return "", errors.Join(
			fmt.Errorf("register authenticated session: %w", err),
			removeSessionErr,
			removeRefErr,
		)
	}

	if oldID != "" && oldID != sess.ID {
		_ = deps.removeSessionRefs(ctx, uid, []string{oldID})
	}

	return sess.ID, nil
}
