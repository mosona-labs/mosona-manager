package passive

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WSClient struct {
	conn          *websocket.Conn
	header        http.Header
	url           string
	mu            sync.RWMutex
	reconnecting  bool
	maxRetries    int
	retryInterval time.Duration
	onReconnect   func()
}

func NewWSClient() *WSClient {
	return &WSClient{
		header:        make(http.Header),
		maxRetries:    -1,
		retryInterval: 5 * time.Second,
	}
}

func (c *WSClient) SetHeader(key, value string) {
	c.header.Set(key, value)
}

func (c *WSClient) SetReconnectConfig(maxRetries int, interval time.Duration) {
	c.maxRetries = maxRetries
	c.retryInterval = interval
}

func (c *WSClient) OnReconnect(fn func()) {
	c.onReconnect = fn
}

func (c *WSClient) Connect(ctx context.Context, url string) error {
	c.url = url
	return c.dial(ctx)
}

func (c *WSClient) dial(ctx context.Context) error {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, c.url, c.header)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	go c.monitorConnection(ctx)
	return nil
}

func (c *WSClient) monitorConnection(ctx context.Context) {
	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			return
		}

		_, _, err := conn.ReadMessage()
		if err != nil {
			c.reconnect(ctx)
			return
		}
	}
}

func (c *WSClient) reconnect(ctx context.Context) {
	c.mu.Lock()
	if c.reconnecting {
		c.mu.Unlock()
		return
	}
	c.reconnecting = true
	c.mu.Unlock()

	retries := 0
	for {
		if c.maxRetries >= 0 && retries >= c.maxRetries {
			break
		}

		time.Sleep(c.retryInterval)

		err := c.dial(ctx)
		if err == nil {
			c.mu.Lock()
			c.reconnecting = false
			c.mu.Unlock()

			if c.onReconnect != nil {
				c.onReconnect()
			}
			return
		}

		retries++
	}

	c.mu.Lock()
	c.reconnecting = false
	c.mu.Unlock()
}

func (c *WSClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

func (c *WSClient) SendMessage(messageType int, data []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn == nil {
		return websocket.ErrCloseSent
	}
	return c.conn.WriteMessage(messageType, data)
}

func (c *WSClient) ReadMessage() (int, []byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn == nil {
		return 0, nil, websocket.ErrCloseSent
	}
	return c.conn.ReadMessage()
}
