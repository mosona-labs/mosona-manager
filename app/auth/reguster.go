package auth

import (
	"github.com/labstack/echo/v4"
	"mosona-manager/_type"
	"mosona-manager/config"
	"mosona-manager/db"
	"mosona-manager/utils"
)

func register(c echo.Context) error {
	username := c.FormValue("username")
	emailAddress := c.FormValue("email")
	password := c.FormValue("password")
	token := c.FormValue("token")

	if username == "" || emailAddress == "" || password == "" || token == "" {
		return c.JSON(400, _type.H{Code: "warning", Msg: "Field is empty"})
	}

	// Captcha
	remoteIp := c.RealIP()
	if ok, err := utils.VerifyCaptcha(token, remoteIp); err != nil || !ok {
		return c.JSON(400, _type.H{Code: "warning", Msg: "Captcha verification failed"})
	}

	// Check Exist
	var exist int
	if err := db.Db.QueryRow(
		"SELECT COUNT(*) FROM users WHERE email=$1",
		emailAddress,
	).Scan(&exist); err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
	}
	if exist > 0 {
		return c.JSON(400, _type.H{Code: "warning", Msg: "Email already registered"})
	}

	// Register
	signature := utils.RandomString(32)
	if _, err := db.Db.Exec(
		"INSERT INTO users (username, email, password, salt) VALUES ($1, $2, $3, $4)",
		username, emailAddress, utils.SHA256(password+signature+config.DynamicConf.Token), signature,
	); err != nil {
		return c.JSON(400, _type.H{Code: "error", Msg: "Registration failed"})
	}

	return c.JSON(200, _type.H{Code: "ok", Msg: "Registration success"})
}
