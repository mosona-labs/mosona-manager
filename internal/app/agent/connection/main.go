package connection

import (
	"sync"

	"github.com/gorilla/websocket"
)

type ManagedConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

var (
	mainConnections = make(map[int64]*ManagedConn)
	mu              sync.RWMutex
)

func MainSet(serverId int64, conn *websocket.Conn) *ManagedConn {
	mu.Lock()
	defer mu.Unlock()
	managed := &ManagedConn{conn: conn}
	mainConnections[serverId] = managed
	return managed
}

func MainGet(serverId int64) (*ManagedConn, bool) {
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

func (c *ManagedConn) WriteMessage(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(messageType, data)
}
