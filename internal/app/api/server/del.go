package aserver

import (
	"fmt"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect/conn"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v5"
)

func del(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	tid, _ := c.Get("tid").(int64)
	serverId, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if tid == 0 || serverId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid server data",
		})
	}

	var name string
	if err := db.Db.QueryRow(
		"SELECT name FROM servers WHERE id=$1 AND team_id=$2",
		serverId, tid,
	).Scan(&name); err != nil || name == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Server not found",
		})
	}
	conn.StopServer(serverId)

	if err := db.DeleteServer(tid, serverId); err != nil {
		_ = conn.ReconcileServer(serverId)
		return utils.ErrorHandler(c, err, "Failed to delete server")
	}
	conn.StopServer(serverId)
	if err := influx.RemoveServerStatus(serverId); err != nil {
		log.Printf("failed to remove deleted server %d status from InfluxDB: %v", serverId, err)
	}

	// Log action
	influx.LogAdd(
		tid, uid, "server", fmt.Sprintf("Delete Server: %s (ID: %d)", name, serverId),
		c.RealIP(), c.Request().UserAgent(), "high",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Server deleted successfully",
	})
}
