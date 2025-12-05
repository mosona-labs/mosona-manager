package moauth

import (
	"mosona-manager/_type"
	"mosona-manager/db"
	"mosona-manager/influx"
	"strconv"

	"github.com/labstack/echo/v4"
)

func add(c echo.Context) error {
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
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

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
