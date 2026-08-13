package msettings

import (
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"os"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func uploadFavicon(c *echo.Context) error {
	if _, err := c.MultipartForm(); err != nil {
		if errors.Is(err, echo.ErrStatusRequestEntityTooLarge) {
			return echo.ErrStatusRequestEntityTooLarge
		}
		return c.JSON(400, _type.H{Code: "invalid", Msg: "Invalid favicon upload"})
	}

	image, err := c.FormFile("image")
	if err != nil || image == nil {
		image, err = c.FormFile("favicon")
	}
	if err != nil || image == nil {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Favicon image is required",
		})
	}
	if image.Size > utils.MaxAvatarBytes {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Favicon image is too large",
		})
	}

	file, err := image.Open()
	if err != nil {
		return utils.ErrorHandler(c, err, "Failed to open favicon image")
	}
	defer func() {
		_ = file.Close()
	}()

	if err = os.MkdirAll("./avatars", 0o755); err != nil {
		return utils.ErrorHandler(c, err, "Failed to prepare favicon storage")
	}

	fileName, err := uuid.NewUUID()
	if err != nil {
		return utils.ErrorHandler(c, err, "Failed to generate favicon filename")
	}
	if err = utils.ConvertAvatar(file, "./avatars", fileName.String()); err != nil {
		return utils.ErrorHandler(c, err, "Failed to process favicon image")
	}

	faviconPath := "/avatars/" + fileName.String() + ".avif"
	oldFavicon := config.ReadDynamicConf().Favicon

	if err = db.SetConfig("favicon", faviconPath); err != nil {
		_ = os.Remove(path.Join("./avatars", path.Base(faviconPath)))
		return utils.ErrorHandler(c, err, "Database error")
	}
	if err = db.SyncConfig(); err != nil {
		return utils.ErrorHandler(c, err, "Failed to reload configuration")
	}

	removeOldFavicon(oldFavicon, faviconPath)

	influx.LogAdd(
		0, c.Get("uid").(int64), "settings", "updated favicon",
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Favicon updated successfully",
		Data: _type.Map{
			"favicon": faviconPath,
		},
	})
}

func removeOldFavicon(oldFavicon, newFavicon string) {
	oldFavicon = strings.TrimSpace(oldFavicon)
	if !strings.HasPrefix(oldFavicon, "/avatars/") {
		return
	}
	oldName := path.Base(oldFavicon)
	if oldName == "" || oldName == "." || oldName == "/" || oldName == path.Base(newFavicon) {
		return
	}
	_ = os.Remove(path.Join("./avatars", oldName))
}
