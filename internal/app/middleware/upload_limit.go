package middleware

import (
	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
)

// AvatarUploadLimit caps the complete multipart request, including form fields
// and encoding overhead, before uploaded files are parsed.
func AvatarUploadLimit(next echo.HandlerFunc) echo.HandlerFunc {
	return echomiddleware.BodyLimit(utils.MaxAvatarRequestBytes)(next)
}
