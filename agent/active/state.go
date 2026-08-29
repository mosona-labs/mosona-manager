package active

import (
	"log"
	"mosona-manager/agent/config"
	"mosona-manager/agent/telemetry"
	agentTypes "mosona-manager/agent/types"
	"mosona-manager/pkg/securews"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

type stateMonitor interface {
	Snapshot() (*agentTypes.Status, error)
}

func handleStateWebSocket(
	conn *websocket.Conn,
	sc *secureWS.SessionCrypto,
) {
	defer func() {
		_ = conn.Close()
	}()
	writer := newWebSocketWriter(conn)

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	if config.Current.NoMonitor {
		<-readDone
		return
	}
	runStateMonitorUntil(writer, sc, telemetry.NewMonitor(), readDone)
}

func runStateMonitor(conn *websocket.Conn, sc *secureWS.SessionCrypto, monitor stateMonitor) {
	var writer *websocketWriter
	if conn != nil {
		writer = newWebSocketWriter(conn)
	}
	runStateMonitorUntil(writer, sc, monitor, nil)
}

func runStateMonitorUntil(writer *websocketWriter, sc *secureWS.SessionCrypto, monitor stateMonitor, done <-chan struct{}) {
	for {
		start := time.Now()

		s, err := monitor.Snapshot()
		if err != nil {
			log.Println("Failed to get status; closing monitoring connection:", err)
			return
		}
		data, err := msgpack.Marshal(s)
		if err != nil {
			log.Println("Failed to marshal status; closing monitoring connection:", err)
			return
		}

		frame, err := sc.Encrypt(data)
		if err != nil {
			log.Println("Failed to encrypt status; closing monitoring connection:", err)
			return
		}
		if err = writer.writeMessage(websocket.BinaryMessage, frame); err != nil {
			return
		}

		sleepFor := 3*time.Second - time.Since(start)
		if sleepFor > 0 {
			timer := time.NewTimer(sleepFor)
			select {
			case <-done:
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}
