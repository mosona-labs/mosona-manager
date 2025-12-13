package agent

import (
	"log"
	agentType "mosona-manager/agent/types"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect"
	"mosona-manager/internal/influx"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/vmihailenco/msgpack/v5"
)

func passiveInfo(c echo.Context) error {
	serverId := c.Get("server_id").(int64)

	system := c.FormValue("system")
	start, _ := strconv.ParseInt(c.FormValue("start_time"), 10, 64)
	hostName := c.FormValue("host_name")
	cpuName := c.FormValue("cpu_name")
	coreC, _ := strconv.Atoi(c.FormValue("core_c"))
	coreT, _ := strconv.Atoi(c.FormValue("core_t"))
	kernel := c.FormValue("kernel")
	arch := c.FormValue("arch")

	connect.CallbackInformation(
		serverId, "", system, time.Unix(start, 0), hostName, cpuName, coreC, coreT, kernel, c.RealIP(), arch,
	)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Information received",
	})
}

func passiveWS(c echo.Context) error {
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
		_ = ws.Close()
	}()

	// Heartbeat
	go func() {
		for {
			if err := ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
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

		var data agentType.Status
		if err = msgpack.Unmarshal(msg, &data); err != nil {
			continue
		}

		if err = influx.AddServerStatus(serverId, _type.ServerStatusType{
			CPU:           data.CPU,
			MemTotalMB:    data.MemTotalMB,
			MemUsedMB:     data.MemUsedMB,
			SwapTotalMB:   data.SwapTotalMB,
			SwapUsedMB:    data.SwapUsedMB,
			DiskTotalGB:   data.DiskTotalGB,
			DiskUsedGB:    data.DiskUsedGB,
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
