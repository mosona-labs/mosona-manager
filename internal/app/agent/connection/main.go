package connection

import (
	"sync"

	"github.com/gorilla/websocket"
)

var (
	mainConnections = make(map[int64]*websocket.Conn)
	mu              sync.RWMutex
)

func MainSet(serverId int64, conn *websocket.Conn) {
	mu.Lock()
	defer mu.Unlock()
	mainConnections[serverId] = conn
}

func MainGet(serverId int64) (*websocket.Conn, bool) {
	mu.RLock()
	defer mu.RUnlock()
	conn, ok := mainConnections[serverId]
	return conn, ok
}

func MainRemove(serverId int64) {
	mu.Lock()
	defer mu.Unlock()
	delete(mainConnections, serverId)
}
