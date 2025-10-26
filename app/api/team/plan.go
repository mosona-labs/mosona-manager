package ateam

import (
	"github.com/labstack/echo/v4"
	"mosona-manager/_type"
	"mosona-manager/db"
)

func plans(c echo.Context) error {
	data, err := db.GetAllTeamPlans()
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: data,
	})
}
