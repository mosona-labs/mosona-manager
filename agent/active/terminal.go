package active

import (
	"encoding/json"
	"log"
	"mosona-manager/agent/shellsession"
	secureWS "mosona-manager/pkg/securews"
	pbTypes "mosona-manager/pkg/types"
	"sync"

	"github.com/gorilla/websocket"
)

func handleTerminalWebSocket(
	conn *websocket.Conn,
	sc *secureWS.SessionCrypto,
) {
	defer func() {
		_ = conn.Close()
	}()
	writer := newWebSocketWriter(conn)

	session, err := shellsession.Start()
	if err != nil {
		log.Printf("Failed to start terminal session: %v", err)
		return
	}
	defer func() {
		_ = session.Close()
	}()

	var once sync.Once
	done := make(chan struct{})
	cleanup := func() {
		once.Do(func() {
			close(done)
			_ = conn.Close()
			_ = session.Close()
		})
	}

	// Handle pty output -> websocket
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := session.Read(buf)
			if err != nil {
				cleanup()
				return
			}

			if n > 0 {
				data, err := sc.Encrypt(buf[:n])
				if err != nil {
					cleanup()
					return
				}
				if err := writer.writeMessage(websocket.BinaryMessage, data); err != nil {
					cleanup()
					return
				}
			}
		}
	}()

	// Handle websocket input -> pty
	go func() {
		for {
			_, originMsg, err := conn.ReadMessage()
			if err != nil {
				cleanup()
				return
			}
			msg, err := sc.Decrypt(originMsg)
			if err != nil {
				cleanup()
				return
			}

			var xtermMsg pbTypes.XTermMessage
			if err := json.Unmarshal(msg, &xtermMsg); err != nil {
				_, _ = session.Write(msg)
			} else {
				switch xtermMsg.Type {
				case "input":
					_, _ = session.Write([]byte(xtermMsg.Data))
				case "resize":
					_ = session.Resize(xtermMsg.Rows, xtermMsg.Cols)
				}
			}
		}
	}()

	<-done
}
