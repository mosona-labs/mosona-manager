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

// HandshakeError preserves the HTTP response when a WebSocket upgrade fails.
type HandshakeError struct {
	StatusCode int
	Status     string
	Err        error
}

func (e *HandshakeError) Error() string {
	message := "websocket handshake failed"
	if e.Err != nil {
		message = e.Err.Error()
	}
	if e.Status == "" {
		return message
	}
	return message + ": " + e.Status
}

func (e *HandshakeError) Unwrap() error {
	return e.Err
}

func IsHandshakeStatus(err error, statusCodes ...int) bool {
	var handshakeErr *HandshakeError
	if !errors.As(err, &handshakeErr) {
		return false
	}
	for _, statusCode := range statusCodes {
		if handshakeErr.StatusCode == statusCode {
			return true
		}
	}
	return false
}

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
