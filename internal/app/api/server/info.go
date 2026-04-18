package aserver

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v5"
)

func info(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)

	serverId, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	if tid == 0 || serverId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid server data",
		})
	}

	data, err := db.GetServerInfo(tid, serverId)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: data,
	})
}
