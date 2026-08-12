package connection

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type sessionInfo struct {
	conn  *websocket.Conn
	timer *time.Timer
	done  chan struct{}
	once  sync.Once
}

var (
	sessionConnections = make(map[string]*sessionInfo)
	sessionMu          sync.RWMutex
)

const sessionTimeout = 30 * time.Second

func UserSet(sessionID string, conn *websocket.Conn) <-chan struct{} {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	if oldInfo, exists := sessionConnections[sessionID]; exists && oldInfo.timer != nil {
		oldInfo.timer.Stop()
	}

	timer := time.AfterFunc(sessionTimeout, func() {
		UserRemove(sessionID)
	})

	sessionConnections[sessionID] = &sessionInfo{
		conn:  conn,
		timer: timer,
		done:  make(chan struct{}),
	}
	return sessionConnections[sessionID].done
}

func UserGet(sessionID string) (*websocket.Conn, bool) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	info, ok := sessionConnections[sessionID]
	if !ok {
		return nil, false
	}

	if info.timer != nil {
		info.timer.Stop()
	}

	return info.conn, true
}

func UserRemove(sessionID string) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	if info, exists := sessionConnections[sessionID]; exists {
		if info.timer != nil {
			info.timer.Stop()
		}
		if info.conn != nil {
			_ = info.conn.Close()
		}
		info.once.Do(func() { close(info.done) })
	}

	delete(sessionConnections, sessionID)
}
