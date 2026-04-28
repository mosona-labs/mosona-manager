package wsutil

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	DefaultPingInterval = 20 * time.Second
	DefaultPingTimeout  = 10 * time.Second
)

func SetSafePingHandler(conn *websocket.Conn, writeMu *sync.Mutex) {
	conn.SetPingHandler(func(appData string) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(DefaultPingTimeout))
	})
}

func StartPing(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, label string) {
	ticker := time.NewTicker(DefaultPingInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				writeMu.Lock()
				err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(DefaultPingTimeout))
				writeMu.Unlock()
				if err != nil {
					if label != "" {
						log.Println(label+":", err)
					}
					_ = conn.Close()
					return
				}
			}
		}
	}()
}
