package middleware

import (
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/email"
	"mosona-manager/internal/utils/store"

	"github.com/labstack/echo/v5"
	"github.com/pquerna/otp/totp"
)

func TwoFA(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		code := c.FormValue("v_code")
		if code == "" {
			code = c.QueryParam("v_code")
		}

		uid := c.Get("uid").(int64)
		user, err := db.GetUserAuthById(uid)
		if err != nil {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Database error: " + err.Error(),
			})
		}
		if user.TOTP == nil || *user.TOTP == "" {
			// Email
			if err = email.VerifyEmailProvider(); errors.Is(err, email.ErrNoEmailProvider) || errors.Is(err, email.ErrEmailProviderNotInit) {
				return next(c)
			}
			if code == "" {
				return c.JSON(401, _type.H{
					Code: "verify",
					Msg:  "Two-factor authentication code required",
				})
			}
			data, ok := store.GetTwoFACodeState(code)
			if !ok || data.UID != uid {
				// Clean up
				store.DeleteTwoFACodeByUID(uid)
				return c.JSON(400, _type.H{
					Code: "error",
					Msg:  "Invalid verification code",
				})
			}
		} else {
			if code == "" {
				return c.JSON(401, _type.H{
					Code: "verify",
					Msg:  "Two-factor authentication code required",
				})
			}
			if !totp.Validate(code, *user.TOTP) {
				return c.JSON(400, _type.H{
					Code: "error",
					Msg:  "Invalid verification code",
				})
			}
		}

		return next(c)
	}
}
