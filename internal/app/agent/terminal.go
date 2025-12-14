package agent

import (
	"log"
	"mosona-manager/internal/app/agent/connection"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

func terminal(c echo.Context) error {
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
	defer func() {
		_ = ws.Close()
		ws = nil
	}()

	userConn, ok := connection.UserGet(sessionID)
	if !ok || userConn == nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte("Terminal session not found or has expired.\n"))
		return err
	}
	defer func() {
		connection.UserRemove(sessionID)
		userConn = nil
	}()

	var once sync.Once
	done := make(chan struct{})

	go func() {
		defer func() {
			once.Do(func() { close(done) })
		}()
		for {
			if ws == nil || userConn == nil {
				return
			}
			mt, message, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if err := userConn.WriteMessage(mt, message); err != nil {
				break
			}
		}
	}()

	go func() {
		defer func() {
			once.Do(func() { close(done) })
		}()
		for {
			if ws == nil || userConn == nil {
				return
			}
			mt, message, err := userConn.ReadMessage()
			if err != nil {
				return
			}
			if err := ws.WriteMessage(mt, message); err != nil {
				break
			}
		}
	}()

	<-done
	return nil
}
