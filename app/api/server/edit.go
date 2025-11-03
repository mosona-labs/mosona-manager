package aserver

import (
	"fmt"
	"mosona-manager/_type"
	"mosona-manager/connect"
	"mosona-manager/db"
	"strconv"

	"github.com/labstack/echo/v4"
)

func edit(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	serverId, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if tid == 0 || serverId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid server data",
		})
	}

	var data _type.ServerFullType
	if err := c.Bind(&data); err != nil {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid request data",
		})
	}

	var lastAllowMonitor bool
	if err := db.Db.QueryRow(
		"SELECT allow_monitor FROM servers WHERE id=$1 AND team_id=$2",
		serverId, tid,
	).Scan(&lastAllowMonitor); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	if err := db.EditServer(tid, serverId, &data); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to edit server",
		})
	}

	// Handle monitoring service
	if lastAllowMonitor != data.AllowMonitor && data.AllowMonitor {
		go func() {
			if err := connect.StartServer(serverId); err != nil {
				fmt.Println("Failed to start server connection:", err)
			}
		}()
	} else if lastAllowMonitor != data.AllowMonitor && !data.AllowMonitor {
		connect.StopServer(serverId)
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Server edited successfully",
	})
}
