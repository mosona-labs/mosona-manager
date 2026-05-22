package passive

import (
	"encoding/json"
	"fmt"
	"log"
	"mosona-manager/agent/shellsession"
	pbTypes "mosona-manager/pkg/types"
	"mosona-manager/pkg/ws"
	"sync"

	"github.com/gorilla/websocket"
)

func terminal(sessionID string) {
	client, err := connectHub(fmt.Sprintf("/api/agent/terminal/%s", sessionID))
	if err != nil {
		log.Printf("Failed to connect terminal session %s: %v", sessionID, err)
		return
	}
	defer func(client *ws.Client) {
		_ = client.Close()
	}(client)

	session, err := shellsession.Start()
	if err != nil {
		log.Printf("Failed to start terminal session %s: %v", sessionID, err)
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
			_ = client.Close()
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
				err = client.SendMessage(websocket.BinaryMessage, buf[:n])
				if err != nil {
					cleanup()
					return
				}
			}
		}
	}()

	// Handle websocket input -> pty
	go func() {
		for {
			_, msg, err := client.ReadMessage()
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
