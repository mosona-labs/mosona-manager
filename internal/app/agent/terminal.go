package agent

import (
	"log"
	"mosona-manager/internal/app/agent/connection"
	"mosona-manager/pkg/wsutil"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
)

func terminal(c *echo.Context) error {
	sessionID := c.Param("session_id")

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

	userConn, ok := connection.UserGet(sessionID)
	if !ok || userConn == nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte("Terminal session not found or has expired.\n"))
		_ = ws.Close()
		return err
	}

	var once sync.Once
	var wsWriteMu sync.Mutex
	var userWriteMu sync.Mutex
	done := make(chan struct{})

	cleanup := func() {
		once.Do(func() {
			close(done)
			_ = ws.Close()
			connection.UserRemove(sessionID)
		})
	}
	wsutil.StartPing(c.Request().Context(), ws, &wsWriteMu, "passive terminal agent websocket ping")
	wsutil.StartPing(c.Request().Context(), userConn, &userWriteMu, "passive terminal browser websocket ping")

	// User WS -> Agent WS
	go func() {
		defer cleanup()
		for {
			mt, message, err := ws.ReadMessage()
			if err != nil {
				return
			}
			userWriteMu.Lock()
			err = userConn.WriteMessage(mt, message)
			userWriteMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	// Agent WS -> User WS
	go func() {
		defer cleanup()
		for {
			mt, message, err := userConn.ReadMessage()
			if err != nil {
				return
			}
			wsWriteMu.Lock()
			err = ws.WriteMessage(mt, message)
			wsWriteMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	<-done
	return nil
}
