package akeys

import (
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect/conn"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v5"
)

func edit(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	if tid == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid team data",
		})
	}

	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid key ID",
		})
	}

	name := c.FormValue("name")
	if name == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Key name cannot be empty",
		})
	}
	password := c.FormValue("password")

	if err := db.UpdateKey(tid, id, name, password); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	go func() {
		rows, err := db.Db.Query(
			"SELECT id FROM servers s JOIN ssh ON s.id = ssh.server_id WHERE type = 0 AND allow_monitor AND ssh.key_id=$1 AND team_id=$2",
			id, tid,
		)
		if err != nil {
			return
		}
		defer func() {
			_ = rows.Close()
		}()

		for rows.Next() {
			var sid int64
			if err = rows.Scan(&sid); err != nil {
				return
			}
			if reconcileErr := conn.ReconcileServer(sid); reconcileErr != nil {
				fmt.Println("Failed to restart server connection:", reconcileErr)
			}
		}
		if err := rows.Err(); err != nil {
			fmt.Println("Failed to list servers for key reconcile:", err)
		}
	}()

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Key updated successfully",
	})
}
