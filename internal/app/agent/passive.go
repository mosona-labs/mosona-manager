package agent

import (
	"log"
	agentTypes "mosona-manager/agent/types"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/app/agent/connection"
	"mosona-manager/internal/connect/callback"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
	"github.com/vmihailenco/msgpack/v5"
)

func passiveInfo(c *echo.Context) error {
	serverId := c.Get("server_id").(int64)

	system := c.FormValue("system")
	start, _ := strconv.ParseInt(c.FormValue("start_time"), 10, 64)
	hostName := c.FormValue("host_name")
	cpuName := c.FormValue("cpu_name")
	coreC, _ := strconv.Atoi(c.FormValue("core_c"))
	coreT, _ := strconv.Atoi(c.FormValue("core_t"))
	kernel := c.FormValue("kernel")
	arch := c.FormValue("arch")
	version := c.FormValue("version")

	callback.Information(
		serverId, "", system, time.Unix(start, 0), hostName, cpuName, coreC, coreT, kernel, c.RealIP(), arch,
	)

	// Update agent info
	if _, err := db.Db.Exec(
		"UPDATE agents SET status = 1, last_ip = $1, last_version = $2, last_seen_at = NOW() WHERE server_id = $3",
		c.RealIP(), version, serverId,
	); err != nil {
		return c.JSON(400, _type.H{
			Code: "err",
			Msg:  "Database error",
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Information received",
	})
}

func agentDisksToServerDisks(agentDisks []agentTypes.DiskInfo) []_type.DiskInfo {
	if len(agentDisks) == 0 {
		return nil
	}
	disks := make([]_type.DiskInfo, len(agentDisks))
	for i, d := range agentDisks {
		disks[i] = _type.DiskInfo{
			MountPoint: d.MountPoint,
			TotalGB:    d.TotalGB,
			UsedGB:     d.UsedGB,
		}
	}
	return disks
}

func passiveWS(c *echo.Context) error {
	serverId := c.Get("server_id").(int64)

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Println("upgrade:", err)
		return err
	}
	defer func() {
		connection.MainRemove(serverId)

		_ = ws.Close()
	}()

	mainConn := connection.MainSet(serverId, ws)

	// Heartbeat
	go func() {
		for {
			if err := mainConn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				log.Println("ping:", err)
				return
			}
			// Ping every 30 seconds
			select {
			case <-c.Request().Context().Done():
				return
			case <-time.After(30 * time.Second):
			}
		}
	}()

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			return nil
		}

		var data agentTypes.Status
		if err = msgpack.Unmarshal(msg, &data); err != nil {
			continue
		}

		if err = influx.AddServerStatus(serverId, _type.ServerStatusType{
			CPU:           data.CPU,
			MemTotalMB:    data.MemTotalMB,
			MemUsedMB:     data.MemUsedMB,
			SwapTotalMB:   data.SwapTotalMB,
			SwapUsedMB:    data.SwapUsedMB,
			Disks:         agentDisksToServerDisks(data.Disks),
			DiskReadKibS:  data.DiskReadKibS,
			DiskWriteKibS: data.DiskWriteKibS,
			DiskReadIOPS:  data.DiskReadIOPS,
			DiskWriteIOPS: data.DiskWriteIOPS,
			RxKibS:        data.RxKibS,
			TxKibS:        data.TxKibS,
			RxTotalMB:     data.RxTotalMB,
			TxTotalMB:     data.TxTotalMB,
			TCPTotal:      data.TCPTotal,
			UDPTotal:      data.UDPTotal,
			Time:          time.Now(),
		}); err != nil {
			log.Println("Failed to add server status:", err)
		}
	}
}
