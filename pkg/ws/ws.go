package ws

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func NewClient() *Client {
	return &Client{
		header:        make(http.Header),
		maxRetries:    -1,
		retryInterval: 5 * time.Second,
	}
}

func (c *Client) SetHeader(key, value string) {
	c.header.Set(key, value)
}

func (c *Client) SetReconnectConfig(maxRetries int, interval time.Duration) {
	c.maxRetries = maxRetries
	c.retryInterval = interval
}

func (c *Client) OnReconnect(fn func()) {
	c.onReconnect = fn
}

func (c *Client) Connect(ctx context.Context, url string) error {
	c.url = url
	c.ctx = ctx
	return c.dial(ctx)
}

func (c *Client) dial(ctx context.Context) error {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, c.url, c.header)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	return nil
}

func (c *Client) reconnect() error {
	c.mu.Lock()
	if c.reconnecting {
		c.mu.Unlock()
		return websocket.ErrCloseSent
	}
	c.reconnecting = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.reconnecting = false
		c.mu.Unlock()
	}()

	retries := 0
	for {
		if c.maxRetries >= 0 && retries >= c.maxRetries {
			return websocket.ErrCloseSent
		}

		time.Sleep(c.retryInterval)

		err := c.dial(c.ctx)
		if err == nil {
			if c.onReconnect != nil {
				c.onReconnect()
			}
			return nil
		}

		retries++
	}
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

func (c *Client) SendMessage(messageType int, data []byte) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return websocket.ErrCloseSent
	}

	err := conn.WriteMessage(messageType, data)
	if err != nil {
		_ = c.reconnect()
	}
	return err
}

func (c *Client) ReadMessage() (int, []byte, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return 0, nil, websocket.ErrCloseSent
	}

	msgType, data, err := conn.ReadMessage()
	if err != nil {
		if reconnectErr := c.reconnect(); reconnectErr == nil {
			c.mu.RLock()
			conn = c.conn
			c.mu.RUnlock()
			if conn != nil {
				return conn.ReadMessage()
			}
		}
	}
	return msgType, data, err
}
