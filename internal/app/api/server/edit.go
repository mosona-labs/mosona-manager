package aserver

import (
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect/conn"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v5"
)

func edit(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)
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

	var typ int16
	var lastAllowMonitor bool
	if err := db.Db.QueryRow(
		"SELECT type, allow_monitor FROM servers WHERE id=$1 AND team_id=$2",
		serverId, tid,
	).Scan(&typ, &lastAllowMonitor); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	if typ == 0 {
		if err := validateSSHConnectionForEdit(tid, serverId, &data); err != nil {
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "SSH connection failed: " + err.Error(),
			})
		}
	}

	if err := db.EditServer(tid, serverId, typ, &data); err != nil {
		return utils.ErrorHandler(c, err, "Failed to edit server")
	}

	// Handle monitoring service
	if typ != 1 && data.AllowMonitor {
		go func() {
			if err := conn.StartServer(serverId, typ); err != nil {
				log.Println("Failed to start server connection:", err)
			}
		}()
	} else if lastAllowMonitor != data.AllowMonitor && !data.AllowMonitor {
		conn.StopServer(serverId)
	}

	// Log action
	influx.LogAdd(
		tid, uid, "server", "Edit Server: "+data.Name+" (ID"+strconv.FormatInt(serverId, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Server edited successfully",
	})
}
