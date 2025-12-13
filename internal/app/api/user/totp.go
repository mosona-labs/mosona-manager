package auser

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"

	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
)

func enableTOTP(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	user, err := db.GetUserAuthById(uid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to get user data",
		})
	}

	if user.TOTP != nil && *user.TOTP != "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "TOTP already enabled",
		})
	}

	token, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Mosona Manager",
		AccountName: user.Email,
	})
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to generate TOTP token",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"secret": token.Secret(),
			"url":    token.URL(),
		},
	})
}

func confirmTOTP(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	user, err := db.GetUserAuthById(uid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to get user data",
		})
	}

	secret := c.FormValue("secret")
	code := c.FormValue("code")

	if secret == "" || code == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Secret and code cannot be empty",
		})
	}
	if user.TOTP != nil && *user.TOTP != "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "TOTP already enabled",
		})
	}

	valid := totp.Validate(code, secret)
	if !valid {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid TOTP code",
		})
	}

	if err = db.SetUserTOTP(uid, &secret); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to enable TOTP",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "TOTP enabled successfully",
	})
}

func disableTOTP(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)

	if err := db.SetUserTOTP(uid, nil); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to disable TOTP",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "TOTP disabled successfully",
	})
}
