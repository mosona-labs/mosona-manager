package utils

import (
	"fmt"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

type OAuthAuthorizationState struct {
	Nonce                    string
	IdentityNamespaceVersion int64
	ConfigRevision           int64
}

func SaveOAuthState(c *echo.Context, providerID int, state, nonce string, identityNamespaceVersion, configRevision int64) error {
	sess, err := session.Get("session", c)
	if err != nil {
		return err
	}

	sess.Values[oauthStateSessionKey(providerID)] = state
	sess.Values[oauthNonceSessionKey(providerID)] = nonce
	sess.Values[oauthNamespaceVersionSessionKey(providerID)] = identityNamespaceVersion
	sess.Values[oauthConfigRevisionSessionKey(providerID)] = configRevision
	return sess.Save(c.Request(), c.Response())
}

func ConsumeOAuthState(c *echo.Context, providerID int, state string) (OAuthAuthorizationState, bool, error) {
	sess, err := session.Get("session", c)
	if err != nil {
		return OAuthAuthorizationState{}, false, err
	}

	key := oauthStateSessionKey(providerID)
	value, ok := sess.Values[key].(string)
	if !ok || value == "" || value != state {
		return OAuthAuthorizationState{}, false, nil
	}
	nonce, _ := sess.Values[oauthNonceSessionKey(providerID)].(string)
	identityNamespaceVersion, versionOK := sess.Values[oauthNamespaceVersionSessionKey(providerID)].(int64)
	configRevision, revisionOK := sess.Values[oauthConfigRevisionSessionKey(providerID)].(int64)

	delete(sess.Values, key)
	delete(sess.Values, oauthNonceSessionKey(providerID))
	delete(sess.Values, oauthNamespaceVersionSessionKey(providerID))
	delete(sess.Values, oauthConfigRevisionSessionKey(providerID))
	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return OAuthAuthorizationState{}, false, err
	}
	if !versionOK || identityNamespaceVersion <= 0 || !revisionOK || configRevision <= 0 {
		return OAuthAuthorizationState{}, false, nil
	}
	return OAuthAuthorizationState{Nonce: nonce, IdentityNamespaceVersion: identityNamespaceVersion, ConfigRevision: configRevision}, true, nil
}

func oauthStateSessionKey(providerID int) string {
	return fmt.Sprintf("oauth_state:%d", providerID)
}

func oauthNonceSessionKey(providerID int) string {
	return fmt.Sprintf("oauth_nonce:%d", providerID)
}

func oauthNamespaceVersionSessionKey(providerID int) string {
	return fmt.Sprintf("oauth_namespace_version:%d", providerID)
}

func oauthConfigRevisionSessionKey(providerID int) string {
	return fmt.Sprintf("oauth_config_revision:%d", providerID)
}
