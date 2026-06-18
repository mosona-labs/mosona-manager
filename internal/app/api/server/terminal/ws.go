package aterminal

import (
	"mosona-manager/internal/connect/active"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/pkg/httporigin"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return httporigin.SameOrigin(r)
	},
}

func ws(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	uid, _ := c.Get("uid").(int64)

	serverId, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	serverAuth, err := db.GetTerminalInfo(tid, serverId)
	if err != nil {
		return err
	}

	// Log action
	influx.LogAdd(
		tid, uid, "terminal", "Access Terminal for Server (ID"+strconv.FormatInt(serverId, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "high",
	)

	wsConn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	// Move ws.Close() to each terminal function to avoid premature close

	if err != nil {
		return err
	}

	if tid == 0 {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte("Invalid team ID.\n"))
		return wsConn.Close()
	}
	if serverId == 0 {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte("Invalid server ID.\n"))
		return wsConn.Close()
	}

	switch serverAuth.Type {
	case 0: // SSH
		if serverAuth.Address == nil || *serverAuth.Address == "" {
			_ = wsConn.WriteMessage(websocket.TextMessage, []byte("Target server not found or terminal access not enabled.\n"))
			return wsConn.Close()
		}
		return terminalSSH(c.Request().Context(), serverAuth, wsConn)
	case 1: // Active Agent
		if err := active.ConnectShell(c.Request().Context(), serverId, wsConn); err != nil {
			return err
		}
		return nil
	case 2: // Passive Agent
		return terminalPassive(serverId, wsConn)
	}

	return nil
}
