package muser

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/security/passwordhash"
	"mosona-manager/internal/utils"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"
)

var errInvalidReauthentication = errors.New("administrator reauthentication failed")

const maxReauthenticationBodyBytes = 8 << 10

func adminMutationForm(request *http.Request) (url.Values, error) {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get(echo.HeaderContentType))
	if err != nil || mediaType != echo.MIMEApplicationForm {
		return nil, errInvalidReauthentication
	}
	data, err := readAdminMutationBody(request)
	if err != nil {
		return nil, err
	}
	values, err := url.ParseQuery(string(data))
	if err != nil {
		return nil, errInvalidReauthentication
	}
	return values, nil
}

func deleteReauthenticationPassword(request *http.Request) (string, error) {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get(echo.HeaderContentType))
	if err != nil {
		return "", errInvalidReauthentication
	}
	data, err := readAdminMutationBody(request)
	if err != nil {
		return "", err
	}

	switch mediaType {
	case echo.MIMEApplicationForm:
		values, err := url.ParseQuery(string(data))
		if err != nil {
			return "", errInvalidReauthentication
		}
		return values.Get("current_password"), nil
	case echo.MIMEApplicationJSON:
		var input struct {
			CurrentPassword string `json:"current_password"`
		}
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return "", errInvalidReauthentication
		}
		return input.CurrentPassword, nil
	default:
		return "", errInvalidReauthentication
	}
}

func readAdminMutationBody(request *http.Request) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(request.Body, maxReauthenticationBodyBytes+1))
	if err != nil || len(data) > maxReauthenticationBodyBytes {
		return nil, errInvalidReauthentication
	}
	return data, nil
}

func reauthenticate(c *echo.Context, actorID int64, currentPassword string) (string, error) {
	if currentPassword == "" {
		return "", errInvalidReauthentication
	}
	user, err := db.GetUserAuthById(actorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", db.ErrActorNotAdmin
		}
		return "", err
	}
	if !user.IsAdmin {
		return "", db.ErrActorNotAdmin
	}
	ok, _, err := passwordhash.Verify(
		currentPassword,
		user.Password,
		user.Salt,
		config.ReadDynamicConf().Token,
	)
	if err != nil || !ok {
		return "", errInvalidReauthentication
	}
	return user.Password, nil
}

func adminMutationError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, errInvalidReauthentication), errors.Is(err, db.ErrAdminReauthenticationRequired):
		return c.JSON(http.StatusUnauthorized, _type.H{
			Code: "reauthentication_required",
			Msg:  "Current administrator password is required",
		})
	case errors.Is(err, db.ErrCannotModifySelf):
		return c.JSON(http.StatusConflict, _type.H{
			Code: "cannot_modify_self",
			Msg:  "Administrators cannot delete or demote themselves",
		})
	case errors.Is(err, db.ErrLastAdmin):
		return c.JSON(http.StatusConflict, _type.H{
			Code: "last_admin_required",
			Msg:  "At least one administrator must remain",
		})
	case errors.Is(err, db.ErrActorNotAdmin):
		return c.JSON(http.StatusForbidden, _type.H{
			Code: "no_admin",
			Msg:  "Administrator permission is required",
		})
	case errors.Is(err, db.ErrUserEmailExists):
		return c.JSON(http.StatusBadRequest, _type.H{
			Code: "warning",
			Msg:  "Email already registered",
		})
	case errors.Is(err, sql.ErrNoRows):
		return c.JSON(http.StatusNotFound, _type.H{
			Code: "user_not_found",
			Msg:  "User not found",
		})
	default:
		return utils.ErrorHandler(c, err, "Database error")
	}
}
