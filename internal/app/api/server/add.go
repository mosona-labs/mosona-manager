package aserver

import (
	"database/sql"
	"fmt"
	"mosona-manager/internal/config"
	"mosona-manager/internal/connect"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"mosona-manager/pkg/_type"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

func add(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	uid, _ := c.Get("uid").(int64)

	name := c.FormValue("name")
	address := c.FormValue("address")
	port, _ := strconv.Atoi(c.FormValue("port"))
	username := c.FormValue("username")
	password := c.FormValue("password")
	keyId := c.FormValue("key_id")
	categoryId, _ := strconv.ParseInt(c.FormValue("category_id"), 10, 64)
	allowMonitor := c.FormValue("allow_monitor") == "true"
	allowTerminal := c.FormValue("allow_terminal") == "true"

	if tid == 0 || name == "" || address == "" || port == 0 || username == "" || categoryId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid server data",
		})
	}

	// Display
	weight, _ := strconv.Atoi(c.FormValue("weight"))
	note := c.FormValue("note")

	// Bill
	provider := c.FormValue("provider")
	cycle, _ := strconv.Atoi(c.FormValue("cycle"))
	startTime := c.FormValue("start_time")
	endTime := c.FormValue("end_time")
	amount := c.FormValue("amount")
	autoRenew := c.FormValue("auto_renew") == "true"

	var startTimeParsed sql.NullTime
	if startTime != "" {
		t, err := time.Parse("2006-01-02", startTime)
		if err != nil {
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Invalid start_time format",
			})
		}
		startTimeParsed = sql.NullTime{Time: t, Valid: true}
	} else {
		startTimeParsed = sql.NullTime{Valid: false}
	}
	var endTimeParsed sql.NullTime
	if endTime != "" {
		t, err := time.Parse("2006-01-02", endTime)
		if err != nil {
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Invalid end_time format",
			})
		}
		endTimeParsed = sql.NullTime{Time: t, Valid: true}
	} else {
		endTimeParsed = sql.NullTime{Valid: false}
	}

	// Network
	bandwidth := c.FormValue("bandwidth")
	traffic := c.FormValue("traffic")
	trafficType, _ := strconv.Atoi(c.FormValue("traffic_type"))
	notePublic := c.FormValue("note_public")

	// Encrypt password
	passwordEncrypt, err := utils.Encrypt([]byte(password), config.Key)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Encryption error",
		})
	}

	// Insert into database
	tx, err := db.Db.Begin()
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	var serverId int64
	if err = tx.QueryRow(`INSERT INTO servers (team_id, name, address, port, username, password, key_id, category, allow_monitor, allow_terminal, weight) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
		tid, name, address, port, username, passwordEncrypt, keyId, categoryId, allowMonitor, allowTerminal, weight,
	).Scan(&serverId); err != nil {
		_ = tx.Rollback()
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	if _, err = tx.Exec(
		"INSERT INTO server_info (sid, note, provider, cycle, start_time, end_time, amount, auto_renew, bandwidth, traffic, traffic_type, note_public) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)",
		serverId, note, provider, cycle, startTimeParsed, endTimeParsed, amount, autoRenew, bandwidth, traffic, trafficType, notePublic,
	); err != nil {
		_ = tx.Rollback()
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	if _, err = tx.Exec(
		"INSERT INTO server_info_adv (sid) VALUES ($1)",
		serverId,
	); err != nil {
		_ = tx.Rollback()
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	if err = tx.Commit(); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	go func() {
		if err = connect.StartServer(serverId); err != nil {
			fmt.Println("Failed to start server connection:", err)
		}
	}()

	// Log action
	influx.LogAdd(
		tid, uid, "server", "Create Server: "+name+" (ID"+strconv.FormatInt(serverId, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "high",
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Server added",
		Data: serverId,
	})
}
