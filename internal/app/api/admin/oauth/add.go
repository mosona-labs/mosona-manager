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
	input, err := (providerInput{
		Name: c.FormValue("name"), Icon: c.FormValue("icon"), Protocol: c.FormValue("protocol"),
		IssuerURL: c.FormValue("issuer_url"), AuthURL: c.FormValue("auth_url"), TokenURL: c.FormValue("token_url"),
		UserinfoURL: c.FormValue("userinfo_url"), Scopes: c.FormValue("scopes"), SubjectField: c.FormValue("subject_field"),
		ClientID: c.FormValue("client_id"), ClientSecret: c.FormValue("client_secret"),
		Skip2FA: c.FormValue("skip_2fa") == "true", IsEnabled: c.FormValue("is_enabled") == "true",
	}).normalize()
	if err != nil {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  err.Error(),
		})
	}

	var providerConfig *oauth.ProviderConfig
	if input.IsEnabled {
		providerConfig, err = oauth.BuildProvider(c.Request().Context(), input.provider(0))
		if err != nil {
			return c.JSON(400, _type.H{Code: "invalid", Msg: "OAuth provider validation failed"})
		}
	}
	id, identityNamespaceVersion, configRevision, err := db.CreateOAuthProvider(input.Name, input.Icon, input.Protocol, input.IssuerURL, input.AuthURL, input.TokenURL, input.UserinfoURL, input.Scopes, input.SubjectField, input.ClientID, input.ClientSecret, input.Skip2FA, input.IsEnabled)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	if input.IsEnabled {
		providerConfig.IdentityNamespaceVersion = identityNamespaceVersion
		providerConfig.ConfigRevision = configRevision
		oauth.InstallProvider(id, providerConfig)
	} else {
		oauth.RemoveProvider(id, configRevision)
	}

	// Log action
	influx.LogAdd(
		0, uid, "oauth", "Add OAuth Provider: "+input.Name+" (ID"+strconv.Itoa(id)+")",
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "OAuth provider added",
		Data: id,
	})
}
