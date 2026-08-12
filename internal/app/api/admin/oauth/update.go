package moauth

import (
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/oauth"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v5"
)

func update(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	id, _ := strconv.Atoi(c.Param("id"))

	if id == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid provider ID",
		})
	}

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
		providerConfig, err = oauth.BuildProvider(c.Request().Context(), input.provider(id))
		if err != nil {
			return c.JSON(400, _type.H{Code: "invalid", Msg: "OAuth provider validation failed"})
		}
	}
	identityNamespaceVersion, configRevision, err := db.UpdateOAuthProvider(id, input.Name, input.Icon, input.Protocol, input.IssuerURL, input.AuthURL, input.TokenURL, input.UserinfoURL, input.Scopes, input.SubjectField, input.ClientID, input.ClientSecret, input.Skip2FA, input.IsEnabled)
	if err != nil {
		if errors.Is(err, db.ErrOAuthIdentityNamespaceLocked) {
			return c.JSON(409, _type.H{Code: "identity_namespace_locked", Msg: err.Error()})
		}
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
		0, uid, "oauth", "Update OAuth Provider: "+input.Name+" (ID"+strconv.Itoa(id)+")",
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "OAuth provider updated",
	})
}
