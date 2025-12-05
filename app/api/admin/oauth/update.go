package moauth

import (
	"mosona-manager/_type"
	"mosona-manager/db"
	"mosona-manager/influx"
	"mosona-manager/oauth"
	"strconv"

	"github.com/labstack/echo/v4"
)

func update(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	id, _ := strconv.Atoi(c.Param("id"))
	name := c.FormValue("name")
	icon := c.FormValue("icon")
	authUrl := c.FormValue("auth_url")
	tokenUrl := c.FormValue("token_url")
	userinfoUrl := c.FormValue("userinfo_url")
	clientID := c.FormValue("client_id")
	clientSecret := c.FormValue("client_secret")
	skip2FA, _ := strconv.ParseBool(c.FormValue("skip_2fa"))
	isEnabled, _ := strconv.ParseBool(c.FormValue("is_enabled"))

	if id == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid provider ID",
		})
	}

	if name == "" || authUrl == "" || tokenUrl == "" || userinfoUrl == "" || clientID == "" || clientSecret == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Missing required fields",
		})
	}

	if err := db.UpdateOAuthProvider(id, name, icon, authUrl, tokenUrl, userinfoUrl, clientID, clientSecret, skip2FA, isEnabled); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	// Update OAuth manager
	oauth.Init()

	// Log action
	influx.LogAdd(
		0, uid, "oauth", "Update OAuth Provider: "+name+" (ID"+strconv.Itoa(id)+")",
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "OAuth provider updated",
	})
}
