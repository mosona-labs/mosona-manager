package muser

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/Masterminds/squirrel"
	"github.com/labstack/echo/v5"
)

func edit(c *echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid user ID",
		})
	}
	username := c.FormValue("username")
	email := c.FormValue("email")
	password := c.FormValue("password")
	verified := c.FormValue("verified") == "true"
	isAdmin := c.FormValue("is_admin") == "true"

	if username == "" || email == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Username, email cannot be empty",
		})
	}

	// Check Exist
	isExist, err := db.CheckEmailExistsExcludeID(email, id)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	if isExist {
		return c.JSON(400, _type.H{Code: "warning", Msg: "Email already registered"})
	}

	// Update User
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	updates := psql.Update("users").Where(squirrel.Eq{"id": id}).SetMap(map[string]interface{}{
		"username": username,
		"email":    email,
		"verified": verified,
		"is_admin": isAdmin,
	})
	if password != "" {
		signature := utils.RandomString(32)
		newPassword := utils.SHA256(password + signature + config.DynamicConf.Token)
		updates = updates.Set("password", newPassword).Set("salt", signature)
	}
	sql, args, _ := updates.ToSql()
	if _, err = db.Db.Exec(sql, args...); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Log action
	influx.LogAdd(
		0, c.Get("uid").(int64), "user", "edit user ID "+strconv.FormatInt(id, 10),
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "User updated successfully",
	})
}
