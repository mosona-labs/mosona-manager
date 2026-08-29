package ws

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReconnectDelayBackoffAndCap(t *testing.T) {
	client := NewClient()
	client.SetReconnectBackoff(-1, 5*time.Second, time.Minute)

	tests := []struct {
		retries int
		base    time.Duration
	}{
		{retries: 0, base: 5 * time.Second},
		{retries: 1, base: 10 * time.Second},
		{retries: 4, base: time.Minute},
		{retries: 10, base: time.Minute},
	}
	for _, tt := range tests {
		delay := client.reconnectDelay(tt.retries)
		min := tt.base - tt.base/5
		max := tt.base + tt.base/5
		if delay < min || delay > max {
			t.Fatalf("retries=%d delay=%v outside [%v,%v]", tt.retries, delay, min, max)
		}
	}
}

func TestDialWebSocketPreservesHandshakeStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := dialWebSocket(
		context.Background(),
		"ws"+strings.TrimPrefix(server.URL, "http"),
		nil,
		"",
	)
	if !IsHandshakeStatus(err, http.StatusNotFound) {
		t.Fatalf("dial error = %v, want handshake status 404", err)
	}
	if !errors.Is(err, websocket.ErrBadHandshake) {
		t.Fatalf("dial error = %v, want websocket.ErrBadHandshake", err)
	}
}

func TestCloseCancelsInitialConnectAndPermanentlyClosesClient(t *testing.T) {
	client := NewClient()
	started := make(chan struct{})
	client.dialContext = func(ctx context.Context, _ string, _ http.Header, _ string) (*websocket.Conn, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}

	connectDone := make(chan error, 1)
	go func() {
		connectDone <- client.Connect(context.Background(), "ws://example.test")
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("initial dial did not start")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-connectDone:
		if !errors.Is(err, ErrClientClosed) {
			t.Fatalf("Connect() error = %v, want ErrClientClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Connect did not stop after Close")
	}
	if err := client.Connect(context.Background(), "ws://example.test"); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("Connect() after Close error = %v, want ErrClientClosed", err)
	}
}

func TestConcurrentReconnectCallersShareOneDial(t *testing.T) {
	client := NewClient()
	client.SetReconnectConfig(1, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.ctx = ctx
	client.cancel = cancel

	started := make(chan struct{})
	release := make(chan struct{})
	var dials atomic.Int32
	client.dialContext = func(context.Context, string, http.Header, string) (*websocket.Conn, error) {
		if dials.Add(1) == 1 {
			close(started)
		}
		<-release
		return nil, errors.New("dial failed")
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- client.reconnect(nil)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("reconnect dial did not start")
	}
	wg.Add(1)
	secondStarted := make(chan struct{})
	go func() {
		defer wg.Done()
		close(secondStarted)
		errs <- client.reconnect(nil)
	}()
	<-secondStarted
	// Give the caller a scheduling turn to join the in-flight reconnect before
	// completing that reconnect. The repeated race run below guards this window.
	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, ErrReconnectExhausted) {
			t.Fatalf("reconnect() error = %v, want ErrReconnectExhausted", err)
		}
	}
	if got := dials.Load(); got != 1 {
		t.Fatalf("dial calls = %d, want 1", got)
	}
}

func TestCloseCancelsReconnectAndPreventsFurtherDials(t *testing.T) {
	client := NewClient()
	client.SetReconnectBackoff(-1, 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	client.ctx = ctx
	client.cancel = cancel

	started := make(chan struct{})
	var dials atomic.Int32
	client.dialContext = func(ctx context.Context, _ string, _ http.Header, _ string) (*websocket.Conn, error) {
		if dials.Add(1) == 1 {
			close(started)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}

	reconnectDone := make(chan error, 1)
	go func() {
		reconnectDone <- client.reconnect(nil)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("reconnect dial did not start")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-reconnectDone:
		if !errors.Is(err, ErrClientClosed) {
			t.Fatalf("reconnect() error = %v, want ErrClientClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reconnect did not stop after Close")
	}
	if err := client.reconnect(nil); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("reconnect() after Close error = %v, want ErrClientClosed", err)
	}
	if got := dials.Load(); got != 1 {
		t.Fatalf("dial calls after Close = %d, want 1", got)
	}
}

func TestReconnectExhaustionIsTerminal(t *testing.T) {
	client := NewClient()
	client.SetReconnectConfig(1, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.ctx = ctx
	client.cancel = cancel
	var dials atomic.Int32
	client.dialContext = func(context.Context, string, http.Header, string) (*websocket.Conn, error) {
		dials.Add(1)
		return nil, errors.New("dial failed")
	}

	if err := client.reconnect(nil); !errors.Is(err, ErrReconnectExhausted) {
		t.Fatalf("reconnect() error = %v, want ErrReconnectExhausted", err)
	}
	if _, err := client.connection(); !errors.Is(err, ErrReconnectExhausted) {
		t.Fatalf("connection() error = %v, want ErrReconnectExhausted", err)
	}
	if err := client.reconnect(nil); !errors.Is(err, ErrReconnectExhausted) {
		t.Fatalf("second reconnect() error = %v, want ErrReconnectExhausted", err)
	}
	if got := dials.Load(); got != 1 {
		t.Fatalf("dial calls after exhaustion = %d, want 1", got)
	}
}

func TestZeroReconnectConfigDisablesReconnect(t *testing.T) {
	client := NewClient()
	client.SetReconnectConfig(0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.ctx = ctx
	client.cancel = cancel
	var dials atomic.Int32
	client.dialContext = func(context.Context, string, http.Header, string) (*websocket.Conn, error) {
		dials.Add(1)
		return nil, errors.New("unexpected dial")
	}

	if err := client.reconnect(nil); !errors.Is(err, ErrReconnectExhausted) {
		t.Fatalf("reconnect() error = %v, want ErrReconnectExhausted", err)
	}
	if got := dials.Load(); got != 0 {
		t.Fatalf("dial calls = %d, want 0", got)
	}
}

func TestReadAndWriteAfterCloseReturnClientClosed(t *testing.T) {
	client := NewClient()
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.ReadMessage(); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("ReadMessage() error = %v, want ErrClientClosed", err)
	}
	if err := client.SendMessage(websocket.BinaryMessage, nil); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("SendMessage() error = %v, want ErrClientClosed", err)
	}
}

func TestPongDeadlineExpiresWithoutPong(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		time.Sleep(400 * time.Millisecond)
	}))
	defer server.Close()

	client := NewClient()
	client.SetReconnectConfig(0, 0)
	if err := client.Connect(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http")); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if err := client.SetPongDeadline(100 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.ReadMessage(); err == nil {
		t.Fatal("ReadMessage() succeeded without a pong")
	}
}

func TestPongDeadlineExtendsOnPong(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for i := 0; i < 3; i++ {
			time.Sleep(50 * time.Millisecond)
			if err = conn.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second)); err != nil {
				return
			}
		}
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("alive"))
	}))
	defer server.Close()

	client := NewClient()
	client.SetReconnectConfig(0, 0)
	if err := client.Connect(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http")); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if err := client.SetPongDeadline(200 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "alive" {
		t.Fatalf("message = %q, want alive", data)
	}
}
