package connection

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type sessionInfo struct {
	serverID int64
	conn     *websocket.Conn
	timer    sessionTimer
	done     chan struct{}
	once     sync.Once
}

type sessionTimer interface {
	Stop() bool
}

type sessionTimerFactory func(time.Duration, func()) sessionTimer

var (
	sessionConnections = make(map[string]*sessionInfo)
	sessionMu          sync.Mutex
)

const sessionTimeout = 30 * time.Second

func UserSet(sessionID string, serverID int64, conn *websocket.Conn) (<-chan struct{}, func()) {
	return userSet(sessionID, serverID, conn, func(timeout time.Duration, callback func()) sessionTimer {
		return time.AfterFunc(timeout, callback)
	})
}

func userSet(sessionID string, serverID int64, conn *websocket.Conn, newTimer sessionTimerFactory) (<-chan struct{}, func()) {
	info := &sessionInfo{
		serverID: serverID,
		conn:     conn,
		done:     make(chan struct{}),
	}

	sessionMu.Lock()
	oldInfo := sessionConnections[sessionID]
	sessionConnections[sessionID] = info
	info.timer = newTimer(sessionTimeout, func() {
		removeUserSession(sessionID, info)
	})
	sessionMu.Unlock()

	if oldInfo != nil {
		closeUserSession(oldInfo)
	}
	return info.done, func() { removeUserSession(sessionID, info) }
}

func UserTake(sessionID string, serverID int64) (*websocket.Conn, func(), bool) {
	sessionMu.Lock()
	info, ok := sessionConnections[sessionID]
	if !ok || info.serverID != serverID {
		sessionMu.Unlock()
		return nil, nil, false
	}

	delete(sessionConnections, sessionID)
	if info.timer != nil {
		info.timer.Stop()
	}
	sessionMu.Unlock()

	return info.conn, func() { closeUserSession(info) }, true
}

func removeUserSession(sessionID string, expected *sessionInfo) {
	sessionMu.Lock()
	info, exists := sessionConnections[sessionID]
	if !exists || info != expected {
		sessionMu.Unlock()
		return
	}
	delete(sessionConnections, sessionID)
	sessionMu.Unlock()

	closeUserSession(info)
}

func closeUserSession(info *sessionInfo) {
	info.once.Do(func() {
		if info.timer != nil {
			info.timer.Stop()
		}
		if info.conn != nil {
			_ = info.conn.Close()
		}
		close(info.done)
	})
}
