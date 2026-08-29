package ws

import (
	"context"
	"errors"
	"fmt"
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
		dialContext:   dialWebSocket,
	}
}

func (c *Client) SetHeader(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.header.Set(key, value)
}

func (c *Client) SetReconnectConfig(maxRetries int, interval time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxRetries = maxRetries
	c.retryInterval = interval
	c.maxRetryDelay = interval
}

func (c *Client) SetReconnectBackoff(maxRetries int, initial, maximum time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxRetries = maxRetries
	c.retryInterval = initial
	c.maxRetryDelay = maximum
}

func (c *Client) OnReconnect(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onReconnect = fn
}

func (c *Client) SetIPPreference(preference string) error {
	if _, err := netutil.NetworkForIPPreference(preference); err != nil {
		return err
	}
	c.mu.Lock()
	c.ipPreference = preference
	c.mu.Unlock()
	return nil
}

func (c *Client) Connect(ctx context.Context, url string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClientClosed
	}
	if c.connecting || c.ctx != nil {
		c.mu.Unlock()
		return errors.New("websocket client is already connected")
	}
	clientCtx, cancel := context.WithCancel(ctx)
	c.url = url
	c.ctx = clientCtx
	c.cancel = cancel
	c.connecting = true
	c.mu.Unlock()

	err := c.dial(clientCtx)
	c.mu.Lock()
	if c.ctx == clientCtx {
		c.connecting = false
		if err != nil && !c.closed {
			c.ctx = nil
			c.cancel = nil
		}
	}
	closed := c.closed
	c.mu.Unlock()
	if err != nil {
		cancel()
	}
	if closed {
		return ErrClientClosed
	}
	return err
}

func (c *Client) dial(ctx context.Context) error {
	c.mu.RLock()
	url := c.url
	header := c.header.Clone()
	ipPreference := c.ipPreference
	dialContext := c.dialContext
	c.mu.RUnlock()

	conn, err := dialContext(ctx, url, header, ipPreference)
	if err != nil {
		if stateErr := c.stateError(ctx); stateErr != nil {
			return stateErr
		}
		return err
	}
	wsutil.SetSafePingHandler(conn, &c.writeMu)
	c.mu.RLock()
	pongWait := c.pongWait
	c.mu.RUnlock()
	if pongWait > 0 {
		if err = setPongDeadline(conn, pongWait); err != nil {
			_ = conn.Close()
			return err
		}
	}

	c.mu.Lock()
	if c.closed || c.ctx != ctx || ctx.Err() != nil {
		closed := c.closed
		c.mu.Unlock()
		_ = conn.Close()
		if closed {
			return ErrClientClosed
		}
		return ctx.Err()
	}
	old := c.conn
	c.conn = conn
	c.mu.Unlock()
	if old != nil && old != conn {
		_ = old.Close()
	}
	return nil
}

func dialWebSocket(ctx context.Context, url string, header http.Header, ipPreference string) (*websocket.Conn, error) {
	dialer := websocket.Dialer{}
	if ipPreference != "" {
		network, err := netutil.NetworkForIPPreference(ipPreference)
		if err != nil {
			return nil, err
		}
		netDialer := &net.Dialer{}
		dialer.NetDialContext = func(ctx context.Context, networkAddr, addr string) (net.Conn, error) {
			return netDialer.DialContext(ctx, network, addr)
		}
	}
	conn, response, err := dialer.DialContext(ctx, url, header)
	if err != nil {
		if response != nil {
			handshakeErr := &HandshakeError{
				StatusCode: response.StatusCode,
				Status:     response.Status,
				Err:        err,
			}
			if response.Body != nil {
				_ = response.Body.Close()
			}
			return nil, handshakeErr
		}
		return nil, err
	}
	return conn, nil
}

func (c *Client) reconnect(failedConn *websocket.Conn) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClientClosed
	}
	if c.ctx == nil {
		c.mu.Unlock()
		return websocket.ErrCloseSent
	}
	if call := c.reconnectCall; call != nil {
		ctx := c.ctx
		c.mu.Unlock()
		return c.waitForReconnect(ctx, call)
	}
	if c.conn != failedConn {
		c.mu.Unlock()
		return nil
	}
	if c.terminalErr != nil {
		err := c.terminalErr
		c.mu.Unlock()
		return err
	}
	call := &reconnectCall{done: make(chan struct{})}
	ctx := c.ctx
	c.reconnectCall = call
	c.conn = nil
	c.mu.Unlock()
	if failedConn != nil {
		_ = failedConn.Close()
	}

	err := c.runReconnect(ctx)
	c.mu.Lock()
	if c.closed {
		err = ErrClientClosed
	}
	call.err = err
	if c.reconnectCall == call {
		c.reconnectCall = nil
		c.terminalErr = err
	}
	close(call.done)
	c.mu.Unlock()
	return err
}

func (c *Client) runReconnect(ctx context.Context) error {
	retries := 0
	var lastErr error
	for {
		c.mu.RLock()
		maxRetries := c.maxRetries
		onReconnect := c.onReconnect
		c.mu.RUnlock()
		if maxRetries >= 0 && retries >= maxRetries {
			if lastErr == nil {
				return ErrReconnectExhausted
			}
			return fmt.Errorf("%w: %v", ErrReconnectExhausted, lastErr)
		}

		delay := c.reconnectDelay(retries)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return c.stateError(ctx)
		case <-timer.C:
		}

		if onReconnect != nil {
			onReconnect()
		}
		if stateErr := c.stateError(ctx); stateErr != nil {
			return stateErr
		}
		err := c.dial(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		retries++
	}
}

func (c *Client) waitForReconnect(ctx context.Context, call *reconnectCall) error {
	select {
	case <-call.done:
		return call.err
	case <-ctx.Done():
		return c.stateError(ctx)
	}
}

func (c *Client) stateError(ctx context.Context) error {
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return ErrClientClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (c *Client) reconnectDelay(retries int) time.Duration {
	c.mu.RLock()
	delay := c.retryInterval
	maximum := c.maxRetryDelay
	c.mu.RUnlock()
	for i := 0; i < retries && delay < maximum; i++ {
		delay *= 2
		if delay > maximum {
			delay = maximum
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
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.connecting = false
	cancel := c.cancel
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		return conn.Close()
	}
	return nil
}

func (c *Client) SendMessage(messageType int, data []byte) error {
	conn, err := c.connection()
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	err = conn.WriteMessage(messageType, data)
	c.writeMu.Unlock()
	if err != nil {
		if reconnectErr := c.reconnect(conn); reconnectErr != nil {
			return reconnectErr
		}
	}
	return err
}

func (c *Client) ReadMessage() (int, []byte, error) {
	for {
		conn, err := c.connection()
		if err != nil {
			return 0, nil, err
		}
		msgType, data, err := conn.ReadMessage()
		if err == nil {
			return msgType, data, nil
		}
		if reconnectErr := c.reconnect(conn); reconnectErr != nil {
			return 0, nil, reconnectErr
		}
	}
}

func (c *Client) SetDeadline(deadline time.Time) error {
	conn, err := c.connection()
	if err != nil {
		return err
	}
	if err = conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	c.writeMu.Lock()
	err = conn.SetWriteDeadline(deadline)
	c.writeMu.Unlock()
	return err
}

// SetPongDeadline requires a pong within wait and extends the deadline for every pong.
func (c *Client) SetPongDeadline(wait time.Duration) error {
	if wait <= 0 {
		return fmt.Errorf("pong wait must be positive")
	}
	conn, err := c.connection()
	if err != nil {
		return err
	}
	if err = setPongDeadline(conn, wait); err != nil {
		return err
	}
	c.mu.Lock()
	c.pongWait = wait
	c.mu.Unlock()
	return nil
}

func setPongDeadline(conn *websocket.Conn, wait time.Duration) error {
	if err := conn.SetReadDeadline(time.Now().Add(wait)); err != nil {
		return err
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wait))
	})
	return nil
}

func (c *Client) connection() (*websocket.Conn, error) {
	for {
		c.mu.RLock()
		if c.closed {
			c.mu.RUnlock()
			return nil, ErrClientClosed
		}
		conn := c.conn
		call := c.reconnectCall
		terminalErr := c.terminalErr
		ctx := c.ctx
		c.mu.RUnlock()
		if conn != nil {
			return conn, nil
		}
		if terminalErr != nil {
			return nil, terminalErr
		}
		if call == nil || ctx == nil {
			return nil, websocket.ErrCloseSent
		}
		if err := c.waitForReconnect(ctx, call); err != nil {
			return nil, err
		}
	}
}
