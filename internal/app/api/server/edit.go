package aserver

import (
	"fmt"
	"mosona-manager/internal/connect"
	db2 "mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	_type2 "mosona-manager/pkg/_type"
	"strconv"

	"github.com/labstack/echo/v4"
)

func edit(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	uid, _ := c.Get("uid").(int64)

	serverId, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if tid == 0 || serverId == 0 {
		return c.JSON(400, _type2.H{
			Code: "error",
			Msg:  "Invalid server data",
		})
	}

	var data _type2.ServerFullType
	if err := c.Bind(&data); err != nil {
		return c.JSON(400, _type2.H{
			Code: "error",
			Msg:  "Invalid request data",
		})
	}

	var lastAllowMonitor bool
	if err := db2.Db.QueryRow(
		"SELECT allow_monitor FROM servers WHERE id=$1 AND team_id=$2",
		serverId, tid,
	).Scan(&lastAllowMonitor); err != nil {
		return c.JSON(500, _type2.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	if err := db2.EditServer(tid, serverId, &data); err != nil {
		return c.JSON(500, _type2.H{
			Code: "error",
			Msg:  "Failed to edit server",
		})
	}

	// Handle monitoring service
	if data.AllowMonitor {
		go func() {
			if err := connect.StartServer(serverId); err != nil {
				fmt.Println("Failed to start server connection:", err)
			}
		}()
	} else if lastAllowMonitor != data.AllowMonitor && !data.AllowMonitor {
		connect.StopServer(serverId)
	}

	// Log action
	influx.LogAdd(
		tid, uid, "server", "Edit Server: "+data.Name+" (ID"+strconv.FormatInt(serverId, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "high",
	)

	return c.JSON(200, _type2.H{
		Code: "ok",
		Msg:  "Server edited successfully",
	})
}
