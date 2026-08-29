package active

import (
	"errors"
	"mosona-manager/agent/config"
	agentTypes "mosona-manager/agent/types"
	"mosona-manager/pkg/securews"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type failingStateMonitor struct{}

func (failingStateMonitor) Snapshot() (*agentTypes.Status, error) {
	return nil, errors.New("snapshot unavailable")
}

func TestRunStateMonitorReturnsOnSnapshotFailure(t *testing.T) {
	runStateMonitor(nil, nil, failingStateMonitor{})
}

func TestStateWebSocketReadsPingAndRepliesWithPong(t *testing.T) {
	oldConfig := config.Current
	config.Current.NoMonitor = true
	t.Cleanup(func() { config.Current = oldConfig })

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(serverDone)
		conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		handleStateWebSocket(conn, (*secureWS.SessionCrypto)(nil))
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	pong := make(chan string, 1)
	conn.SetPongHandler(func(data string) error {
		pong <- data
		return nil
	})
	readDone := make(chan error, 1)
	go func() {
		_, _, readErr := conn.ReadMessage()
		readDone <- readErr
	}()
	if err = conn.WriteControl(websocket.PingMessage, []byte("probe"), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-pong:
		if got != "probe" {
			t.Fatalf("pong = %q, want probe", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Agent state WebSocket did not process the ping")
	}
	_ = conn.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("state WebSocket read did not stop after close")
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("state WebSocket handler did not stop after close")
	}
}

func TestWebSocketWriterSerializesDataAndPongWrites(t *testing.T) {
	serverDone := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = conn.Close() }()
		writer := newWebSocketWriter(conn)
		readDone := make(chan error, 1)
		go func() {
			for {
				if _, _, readErr := conn.ReadMessage(); readErr != nil {
					readDone <- readErr
					return
				}
			}
		}()
		for i := 0; i < 200; i++ {
			if err = writer.writeMessage(websocket.BinaryMessage, []byte("status-or-pty")); err != nil {
				serverDone <- err
				return
			}
		}
		_ = conn.Close()
		<-readDone
		serverDone <- nil
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	readDone := make(chan error, 1)
	go func() {
		for {
			if _, _, readErr := conn.ReadMessage(); readErr != nil {
				readDone <- readErr
				return
			}
		}
	}()
	for i := 0; i < 200; i++ {
		if err = conn.WriteControl(websocket.PingMessage, []byte("probe"), time.Now().Add(time.Second)); err != nil {
			break
		}
	}
	select {
	case err = <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent data and pong writes did not finish")
	}
	_ = conn.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("client read did not stop")
	}
}
