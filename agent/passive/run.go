package passive

import (
	"log"
	"mosona-manager/agent/config"
	"mosona-manager/agent/telemetry"
	pbTypes "mosona-manager/pkg/types"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

func Run() {
	if err := config.LoadPrivateKey(); err != nil {
		log.Fatalln("Failed to load private key:", err)
	}

	if err := reportInfo(); err != nil {
		log.Fatalln("Failed to report info:", err)
	}

	client, err := connectHub("/api/agent/ws")
	if err != nil {
		log.Fatalln("Failed to connect to hub:", err)
	}

	// Monitoring loop
	if !config.Current.NoMonitor {
		go func() {
			monitor := telemetry.NewMonitor()
			for {
				start := time.Now()

				s, err := monitor.Snapshot()
				if err != nil {
					log.Fatalln("Failed to get status:", err)
				}
				data, err := msgpack.Marshal(s)
				if err != nil {
					log.Fatalln("Failed to marshal status:", err)
				}

				if err = client.SendMessage(websocket.BinaryMessage, data); err != nil {
					log.Fatalln("Failed to send status:", err)
				}

				sleepFor := 3*time.Second - time.Since(start)
				if sleepFor > 0 {
					time.Sleep(sleepFor)
				}
			}
		}()
	}

	for {
		msgType, data, err := client.ReadMessage()
		if err != nil {
			continue
		}

		if msgType == websocket.BinaryMessage {
			var msg pbTypes.Msg
			if err := msgpack.Unmarshal(data, &msg); err != nil {
				continue
			}

			switch msg.Code {
			case "terminal":
				go terminal(string(msg.Data))
			}
		}
	}
}
