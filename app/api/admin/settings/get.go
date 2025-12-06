package msettings

import (
	"mosona-manager/_type"
	"mosona-manager/config"

	"github.com/labstack/echo/v4"
)

func get(c echo.Context) error {
	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: Response{
			Domain:                  config.DynamicConf.Domain,
			EmailProvider:           config.DynamicConf.EmailProvider,
			SMTPHost:                config.DynamicConf.SMTPHost,
			SMTPPort:                config.DynamicConf.SMTPPort,
			SMTPUsername:            config.DynamicConf.SMTPUsername,
			SMTPPassword:            config.DynamicConf.SMTPPassword,
			SMTPTls:                 config.DynamicConf.SMTPTls,
			EmailVerifyLogin:        config.DynamicConf.EmailVerifyLogin,
			RegistrationEnabled:     config.DynamicConf.RegistrationEnabled,
			RegistrationVerifyEmail: config.DynamicConf.RegistrationVerifyEmail,
			CaptchaSiteKey:          config.DynamicConf.CaptchaSiteKey,
			CaptchaSecretKey:        config.DynamicConf.CaptchaSecret,
		},
	})
}
