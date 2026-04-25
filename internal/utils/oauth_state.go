package utils

import (
	"fmt"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func SaveOAuthState(c *echo.Context, providerID int, state string) error {
	sess, err := session.Get("session", c)
	if err != nil {
		return err
	}

	sess.Values[oauthStateSessionKey(providerID)] = state
	return sess.Save(c.Request(), c.Response())
}

func ConsumeOAuthState(c *echo.Context, providerID int, state string) (bool, error) {
	sess, err := session.Get("session", c)
	if err != nil {
		return false, err
	}

	key := oauthStateSessionKey(providerID)
	value, ok := sess.Values[key].(string)
	if !ok || value == "" || value != state {
		return false, nil
	}

	delete(sess.Values, key)
	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return false, err
	}
	return true, nil
}

func oauthStateSessionKey(providerID int) string {
	return fmt.Sprintf("oauth_state:%d", providerID)
}
