package muser

import (
	"mosona-manager/_type"
	"mosona-manager/config"
	"mosona-manager/db"
	"mosona-manager/utils"

	"github.com/labstack/echo/v4"
)

func add(c echo.Context) error {
	username := c.FormValue("username")
	email := c.FormValue("email")
	password := c.FormValue("password")
	verified := c.FormValue("verified") == "true"
	isAdmin := c.FormValue("is_admin") == "true"

	if username == "" || email == "" || password == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Username, email, and password cannot be empty",
		})
	}

	// Check Exist
	isExist, err := db.CheckEmailExists(email)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
	}
	if isExist {
		return c.JSON(400, _type.H{Code: "warning", Msg: "Email already registered"})
	}

	// Salt
	signature := utils.RandomString(32)
	newPassword := utils.SHA256(password + signature + config.DynamicConf.Token)
	if _, err = db.Db.Exec(
		"INSERT INTO users (username, email, password, salt, verified, is_admin) VALUES ($1, $2, $3, $4, $5, $6)",
		username, email, newPassword, signature, verified, isAdmin,
	); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "User added successfully",
	})
}
