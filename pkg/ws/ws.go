package ws

import (
	"context"
	"math/rand/v2"
	"mosona-manager/pkg/netutil"
	"mosona-manager/pkg/wsutil"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func NewClient() *Client {
	return &Client{
		header:        make(http.Header),
		maxRetries:    -1,
		retryInterval: 5 * time.Second,
		maxRetryDelay: 5 * time.Second,
	}
}

func (c *Client) SetHeader(key, value string) {
	c.header.Set(key, value)
}

func (c *Client) SetReconnectConfig(maxRetries int, interval time.Duration) {
	c.maxRetries = maxRetries
	c.retryInterval = interval
	c.maxRetryDelay = interval
}

func (c *Client) SetReconnectBackoff(maxRetries int, initial, maximum time.Duration) {
	c.maxRetries = maxRetries
	c.retryInterval = initial
	c.maxRetryDelay = maximum
}

func (c *Client) OnReconnect(fn func()) {
	c.onReconnect = fn
}

func (c *Client) SetIPPreference(preference string) error {
	if _, err := netutil.NetworkForIPPreference(preference); err != nil {
		return err
	}
	c.ipPreference = preference
	return nil
}

func (c *Client) Connect(ctx context.Context, url string) error {
	c.url = url
	c.ctx = ctx
	return c.dial(ctx)
}

func (c *Client) dial(ctx context.Context) error {
	dialer := websocket.Dialer{}
	if c.ipPreference != "" {
		network, err := netutil.NetworkForIPPreference(c.ipPreference)
		if err != nil {
			return err
		}
		netDialer := &net.Dialer{}
		dialer.NetDialContext = func(ctx context.Context, networkAddr, addr string) (net.Conn, error) {
			return netDialer.DialContext(ctx, network, addr)
		}
	}
	conn, _, err := dialer.DialContext(ctx, c.url, c.header)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	wsutil.SetSafePingHandler(conn, &c.writeMu)

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

		delay := c.reconnectDelay(retries)
		timer := time.NewTimer(delay)
		select {
		case <-c.ctx.Done():
			timer.Stop()
			return c.ctx.Err()
		case <-timer.C:
		}

		if c.onReconnect != nil {
			c.onReconnect()
		}
		err := c.dial(c.ctx)
		if err == nil {
			return nil
		}

		retries++
	}
}

func (c *Client) reconnectDelay(retries int) time.Duration {
	delay := c.retryInterval
	for i := 0; i < retries && delay < c.maxRetryDelay; i++ {
		delay *= 2
		if delay > c.maxRetryDelay {
			delay = c.maxRetryDelay
		}
	}
	if delay <= 0 {
		return 0
	}
	// Add +/-20% jitter so many agents do not reconnect simultaneously.
	window := delay / 5
	if window == 0 {
		return delay
	}
	return delay - window + time.Duration(rand.Int64N(int64(window*2)+1))
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

	c.writeMu.Lock()
	err := conn.WriteMessage(messageType, data)
	c.writeMu.Unlock()
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
