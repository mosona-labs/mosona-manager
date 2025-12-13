package auser

import (
	"database/sql"
	"encoding/json"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/oauth"
	"mosona-manager/internal/utils/store"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

func oauthIdentities(c echo.Context) error {
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

func oauthRevoke(c echo.Context) error {
	uid := c.Get("uid").(int64)
	provider, _ := strconv.Atoi(c.Param("id"))
	if provider == 0 {
		return c.JSON(400, _type.H{
			Code: "empty",
			Msg:  "Provider ID cannot be empty",
		})
	}

	if err := db.DeleteAuthIdentityByProviderAndUserID(provider, uid); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
	})
}

func oauthLink(c echo.Context) error {
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
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
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

	// State validation
	expireTime, ok := store.GetAuthSessionState(state)
	if ok && time.Now().After(expireTime) {
		ok = false
	}
	if ok {
		store.DeleteAuthSessionState(state)
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Not Available",
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

	// Is link
	_, err = db.GetBindByProviderAndSubject(oauthID, strconv.FormatInt(profile.ID, 10))
	if err == nil {
		return c.JSON(400, _type.H{
			Code: "exists",
			Msg:  "This OAuth account is already linked to another user",
		})
	} else if !errors.Is(err, sql.ErrNoRows) {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	// Link
	if _, err = db.AddAuthIdentity(uid, oauthID, strconv.FormatInt(profile.ID, 10), profile.Email, profile.Name); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "OAuth account successfully linked to your user account",
	})
}
