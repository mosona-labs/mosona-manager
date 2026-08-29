package active

import (
	"mosona-manager/pkg/wsutil"
	"sync"

	"github.com/gorilla/websocket"
)

type websocketWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func newWebSocketWriter(conn *websocket.Conn) *websocketWriter {
	w := &websocketWriter{conn: conn}
	wsutil.SetSafePingHandler(conn, &w.mu)
	return w
}

func (w *websocketWriter) writeMessage(messageType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(messageType, data)
}
