package aterminal

import (
	"context"
	"errors"
	"mosona-manager/internal/access"
	"mosona-manager/internal/connect/active"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/pkg/httporigin"
	"net/http"
	"strconv"
	"time"

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
	sid, _ := c.Get("sid").(string)

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
	ctx, cancel := context.WithCancel(c.Request().Context())
	defer cancel()
	go func() {
		err, ok := <-access.WatchTeamSession(ctx, 2*time.Second, uid, tid, sid, 0, 1)
		if !ok || err == nil {
			return
		}
		reason := "authorization check failed"
		if errors.Is(err, access.ErrTeamAccessRevoked) {
			reason = "team access revoked"
		}
		_ = wsConn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, reason),
			time.Now().Add(time.Second),
		)
		cancel()
	}()

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
		return terminalSSH(ctx, serverAuth, wsConn, tid, uid, serverId, c.RealIP(), c.Request().UserAgent())
	case 1: // Active Agent
		if err := active.ConnectShell(ctx, serverId, wsConn); err != nil {
			return err
		}
		return nil
	case 2: // Passive Agent
		return terminalPassive(ctx, serverId, wsConn)
	}

	return nil
}
