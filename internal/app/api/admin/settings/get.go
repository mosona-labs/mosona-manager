package msettings

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"

	"github.com/labstack/echo/v4"
)

func get(c echo.Context) error {
	dc := config.ReadDynamicConf()

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: Response{
			Domain:                  dc.Domain,
			EmailProvider:           dc.EmailProvider,
			SMTPHost:                dc.SMTPHost,
			SMTPPort:                dc.SMTPPort,
			SMTPUsername:            dc.SMTPUsername,
			SMTPPassword:            dc.SMTPPassword,
			SMTPTls:                 dc.SMTPTls,
			EmailVerifyLogin:        dc.EmailVerifyLogin,
			RegistrationEnabled:     dc.RegistrationEnabled,
			RegistrationVerifyEmail: dc.RegistrationVerifyEmail,
			CaptchaSiteKey:          dc.CaptchaSiteKey,
			CaptchaSecretKey:        dc.CaptchaSecret,
		},
	})
}
