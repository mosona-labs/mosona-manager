package ws

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	ErrClientClosed       = errors.New("websocket client is closed")
	ErrReconnectExhausted = errors.New("websocket reconnect attempts exhausted")
)

type reconnectCall struct {
	done chan struct{}
	err  error
}

type dialContextFunc func(context.Context, string, http.Header, string) (*websocket.Conn, error)

type Client struct {
	conn          *websocket.Conn
	header        http.Header
	url           string
	mu            sync.RWMutex
	writeMu       sync.Mutex
	reconnectCall *reconnectCall
	terminalErr   error
	maxRetries    int
	retryInterval time.Duration
	maxRetryDelay time.Duration
	pongWait      time.Duration
	onReconnect   func()
	ctx           context.Context
	cancel        context.CancelFunc
	closed        bool
	connecting    bool
	ipPreference  string
	dialContext   dialContextFunc
}
