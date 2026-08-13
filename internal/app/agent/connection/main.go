package connection

import (
	"context"
	"mosona-manager/internal/connect/conn"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

func init() {
	conn.RegisterInboundStopper(MainClose)
}

type ManagedConn struct {
	conn         *websocket.Conn
	writeMu      sync.Mutex
	stateMu      sync.RWMutex
	closed       atomic.Bool
	closeFn      func() error
	statusCtx    context.Context
	cancelStatus context.CancelFunc
}

var (
	mainConnections = make(map[int64]*ManagedConn)
	mu              sync.RWMutex
)

func MainSet(serverId int64, conn *websocket.Conn) *ManagedConn {
	statusCtx, cancelStatus := context.WithCancel(context.Background())
	managed := &ManagedConn{
		conn:         conn,
		closeFn:      conn.Close,
		statusCtx:    statusCtx,
		cancelStatus: cancelStatus,
	}
	mainSetManaged(serverId, managed)
	return managed
}

func mainSetManaged(serverId int64, managed *ManagedConn) {
	mu.Lock()
	old := mainConnections[serverId]
	mainConnections[serverId] = managed
	mu.Unlock()
	if old != nil {
		old.Close()
	}
}

func MainGet(serverId int64) (*ManagedConn, bool) {
	mu.RLock()
	defer mu.RUnlock()
	conn, ok := mainConnections[serverId]
	return conn, ok
}

func MainRemove(serverId int64, managed *ManagedConn) {
	mu.Lock()
	defer mu.Unlock()
	if mainConnections[serverId] == managed {
		delete(mainConnections, serverId)
	}
}

func MainClose(serverId int64) {
	mu.Lock()
	managed, ok := mainConnections[serverId]
	if ok {
		delete(mainConnections, serverId)
	}
	mu.Unlock()
	if ok {
		managed.Close()
	}
}

func (c *ManagedConn) Close() {
	if c.closed.CompareAndSwap(false, true) {
		c.cancelStatus()
		_ = c.closeFn()
	}
	c.stateMu.Lock()
	c.stateMu.Unlock()
}

func (c *ManagedConn) Closed() bool {
	return c.closed.Load()
}

func (c *ManagedConn) WhileOpen(fn func(context.Context)) bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.closed.Load() {
		return false
	}
	fn(c.statusCtx)
	return true
}

func (c *ManagedConn) WriteMessage(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(messageType, data)
}

func (c *ManagedConn) WriteMutex() *sync.Mutex {
	return &c.writeMu
}
