package moauth

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/oauth"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v5"
)

func add(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	name := c.FormValue("name")
	icon := c.FormValue("icon")
	authUrl := c.FormValue("auth_url")
	tokenUrl := c.FormValue("token_url")
	userinfoUrl := c.FormValue("userinfo_url")
	clientID := c.FormValue("client_id")
	clientSecret := c.FormValue("client_secret")
	skip2FA := c.FormValue("skip_2fa") == "true"
	isEnabled := c.FormValue("is_enabled") == "true"

	if name == "" || authUrl == "" || tokenUrl == "" || userinfoUrl == "" || clientID == "" || clientSecret == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Missing required fields",
		})
	}

	id, err := db.CreateOAuthProvider(name, icon, authUrl, tokenUrl, userinfoUrl, clientID, clientSecret, skip2FA, isEnabled)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Add to OAuth manager
	oauth.AddProvider(_type.AuthProvider{
		ID:           id,
		Name:         name,
		Icon:         icon,
		AuthUrl:      authUrl,
		TokenUrl:     tokenUrl,
		UserinfoUrl:  userinfoUrl,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Skip2FA:      skip2FA,
		IsEnabled:    isEnabled,
	})

	// Log action
	influx.LogAdd(
		0, uid, "oauth", "Add OAuth Provider: "+name+" (ID"+strconv.Itoa(id)+")",
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "OAuth provider added",
		Data: id,
	})
}
