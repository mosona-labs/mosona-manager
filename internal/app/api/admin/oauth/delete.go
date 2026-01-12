package moauth

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/oauth"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v4"
)

func del(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	id, _ := strconv.Atoi(c.Param("id"))

	if id == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid provider ID",
		})
	}

	// Get provider info before deletion for logging
	provider, err := db.GetAuthProviderByID(id)
	if err != nil {
		return utils.ErrorHandler(c, err, "Provider not found")
	}

	if err = db.DeleteOAuthProvider(id); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Remove from OAuth manager
	oauth.RemoveProvider(id)

	// Log action
	influx.LogAdd(
		0, uid, "oauth", "Delete OAuth Provider: "+provider.Name+" (ID"+strconv.Itoa(id)+")",
		c.RealIP(), c.Request().UserAgent(), "high",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "OAuth provider deleted",
	})
}
