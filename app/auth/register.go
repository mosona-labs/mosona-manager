package auth

import (
	"mosona-manager/_type"
	"mosona-manager/config"
	"mosona-manager/db"
	"mosona-manager/utils"

	"github.com/labstack/echo/v4"
)

func register(c echo.Context) error {
	if config.DynamicConf.RegistrationEnabled {
		return c.JSON(403, _type.H{Code: "forbidden", Msg: "Registration is disabled"})
	}

	username := c.FormValue("username")
	emailAddress := c.FormValue("email")
	password := c.FormValue("password")
	token := c.FormValue("token")

	if username == "" || emailAddress == "" || password == "" || token == "" {
		return c.JSON(400, _type.H{Code: "warning", Msg: "Field is empty"})
	}

	// Captcha
	if config.DynamicConf.CaptchaSecret != "" && config.DynamicConf.CaptchaSiteKey != "" {
		remoteIp := c.RealIP()
		if ok, err := utils.VerifyCaptcha(token, remoteIp); err != nil || !ok {
			return c.JSON(400, _type.H{Code: "warning", Msg: "Captcha verification failed"})
		}
	}

	// Check Exist
	isExist, err := db.CheckEmailExists(emailAddress)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
	}
	if isExist {
		return c.JSON(400, _type.H{Code: "warning", Msg: "Email already registered"})
	}

	// Register
	signature := utils.RandomString(32)
	if _, err := db.Db.Exec(
		"INSERT INTO users (username, email, password, salt, verified) VALUES ($1, $2, $3, $4, $5)",
		username, emailAddress, utils.SHA256(password+signature+config.DynamicConf.Token), signature, !config.DynamicConf.RegistrationVerifyEmail,
	); err != nil {
		return c.JSON(400, _type.H{Code: "error", Msg: "Registration failed"})
	}

	return c.JSON(200, _type.H{Code: "ok", Msg: "Registration success"})
}
