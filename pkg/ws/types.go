package ws

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	conn          *websocket.Conn
	header        http.Header
	url           string
	mu            sync.RWMutex
	writeMu       sync.Mutex
	reconnecting  bool
	maxRetries    int
	retryInterval time.Duration
	maxRetryDelay time.Duration
	onReconnect   func()
	ctx           context.Context
	ipPreference  string
}
