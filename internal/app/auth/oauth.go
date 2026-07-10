package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/app/middleware"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/oauth"
	"mosona-manager/internal/utils"
	"mosona-manager/internal/utils/store"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
	"golang.org/x/oauth2"
)

func oauthLogin(c *echo.Context) error {
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

	// Generate state parameter
	state := utils.RandomString(32)
	store.SetAuthSessionState(state)
	if err := utils.SaveOAuthState(c, oauthID, state); err != nil {
		store.DeleteAuthSessionState(state)
		return c.JSON(500, _type.H{Code: "error", Msg: "Session save failed"})
	}

	authURL := cfg.Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: _type.Map{
			"url":   authURL,
			"state": state,
		},
	})
}

func oauthCallback(c *echo.Context) error {
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

	ok, err := utils.ConsumeOAuthState(c, oauthID, state)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session update failed"})
	}
	if !ok || !store.ConsumeAuthSessionState(state, time.Now()) {
		if ok {
			store.DeleteAuthSessionState(state)
		}
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid or expired OAuth state",
		})
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
	client := cfg.Config.Client(ctx, token)

	// Get Userinfo
	resp, err := client.Get(cfg.UserinfoUrl)
	if err != nil {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Failed to get user info",
		})
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Claims
	var profile struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Failed to parse ID Token claims: " + err.Error(),
		})
	}

	identity, err := db.GetAuthIdentityBySubject(oauthID, fmt.Sprint(profile.ID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(404, _type.H{
				Code: "not_found",
				Msg:  "OAuth identity not linked, please bind your account first.",
			})
		}
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	user, err := db.GetUserById(identity.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(404, _type.H{
				Code: "error",
				Msg:  "User not found",
			})
		}
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	// Session
	sess, err := session.Get("session", c)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session init error"})
	}
	sess.Options = sessionOptions(86400 * 3)

	// Already logged in
	if sess.Values["uid"] != nil {
		currentUID, ok := sess.Values["uid"].(int64)
		if ok && currentUID > 0 && middleware.SessionBindingOK(c, sess) {
			return c.JSON(500, _type.H{Code: "error", Msg: "Already logged in"})
		}
	}

	// MFA & TOTP Check
	if (user.TOTP != nil && *user.TOTP) || config.DynamicConf.EmailVerifyLogin {
		provider, err := db.GetAuthProviderByID(identity.ProviderID)
		if err != nil {
			return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
		}
		if !provider.Skip2FA {
			sess.Values["pre_2fa_uid"] = user.ID
			if err = sess.Save(c.Request(), c.Response()); err != nil {
				return c.JSON(500, _type.H{Code: "error", Msg: "Session save failed"})
			}
			// 2FA Required
			return c.JSON(200, _type.H{Code: "2fa_required", Msg: "Two-factor authentication required"})
		}
	}

	sessID, err := finalizeAuthenticatedSession(c, user.ID, 86400*3)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session save failed"})
	}

	loginEvent(user.ID, sessID, c.RealIP(), c.Request().Header.Get("User-Agent"), user.IsAdmin)

	return c.JSON(200, _type.H{Code: "ok", Msg: "Success"})
}
