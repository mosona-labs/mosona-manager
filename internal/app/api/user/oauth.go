package auser

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/oauth"
	"mosona-manager/internal/utils"
	"mosona-manager/internal/utils/store"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

func oauthIdentities(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)

	data, err := db.GetAuthByUserID(uid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: data,
	})
}

func oauthRevoke(c *echo.Context) error {
	uid := c.Get("uid").(int64)
	provider, _ := strconv.Atoi(c.Param("id"))
	if provider == 0 {
		return c.JSON(400, _type.H{
			Code: "empty",
			Msg:  "Provider ID cannot be empty",
		})
	}

	if err := db.DeleteAuthIdentityByProviderAndUserID(provider, uid); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
	})
}

func oauthLink(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)

	oauthID, _ := strconv.Atoi(c.Param("id"))
	if oauthID == 0 {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid OAuth ID",
		})
	}
	cfg, ok := oauth.GetProviders(oauthID)
	if !ok {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid OAuth provider",
		})
	}

	// Check user link
	_, err := db.GetBindByProviderAndUserID(oauthID, uid)
	if err == nil {
		return c.JSON(400, _type.H{
			Code: "exists",
			Msg:  "This OAuth provider is already linked to your account",
		})
	} else if !errors.Is(err, sql.ErrNoRows) {
		return utils.ErrorHandler(c, err, "Database error")
	}

	code := c.FormValue("code")
	state := c.FormValue("state")
	if code == "" || state == "" {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Code or state parameter is missing",
		})
	}

	// Context
	ctx := c.Request().Context()

	authorizationState, ok, err := utils.ConsumeOAuthState(c, oauthID, state)
	if err != nil {
		return utils.ErrorHandler(c, err, "Session update failed")
	}
	if !ok || !store.ConsumeAuthSessionState(state, oauthID, authorizationState.ConfigRevision, time.Now()) {
		if ok {
			store.DeleteAuthSessionState(state)
		}
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid or expired OAuth state",
		})
	}
	if cfg.IdentityNamespaceVersion != authorizationState.IdentityNamespaceVersion || cfg.ConfigRevision != authorizationState.ConfigRevision {
		return c.JSON(409, _type.H{Code: "identity_namespace_changed", Msg: "OAuth provider configuration changed; restart authorization"})
	}

	token, err := cfg.Config.Exchange(ctx, code)
	if err != nil {
		if strings.Contains(err.Error(), "bad_verification_code") {
			return c.JSON(400, _type.H{
				Code: "invalid",
				Msg:  "Authorization code is incorrect or expired",
			})
		}
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Failed to exchange code for token: " + err.Error(),
		})
	}
	profile, err := cfg.Identity(ctx, token, authorizationState.Nonce)
	if err != nil {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "OAuth provider returned an invalid user identity",
		})
	}

	// Is link
	_, err = db.GetBindByProviderAndSubject(oauthID, profile.Subject)
	if err == nil {
		return c.JSON(400, _type.H{
			Code: "exists",
			Msg:  "This OAuth account is already linked to another user",
		})
	} else if !errors.Is(err, sql.ErrNoRows) {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Link
	if _, err = db.AddAuthIdentity(uid, oauthID, authorizationState.IdentityNamespaceVersion, authorizationState.ConfigRevision, profile.Subject, profile.Email, profile.Name); err != nil {
		if errors.Is(err, db.ErrOAuthIdentityNamespaceChanged) {
			return c.JSON(409, _type.H{Code: "identity_namespace_changed", Msg: "OAuth provider configuration changed; restart authorization"})
		}
		if errors.Is(err, db.ErrOAuthIdentityAlreadyLinked) {
			return c.JSON(409, _type.H{Code: "exists", Msg: "This OAuth account or provider is already linked"})
		}
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "OAuth account successfully linked to your user account",
	})
}
