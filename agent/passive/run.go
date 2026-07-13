package passive

import (
	"log"
	"math/rand/v2"
	"mosona-manager/agent/config"
	"mosona-manager/agent/telemetry"
	pbTypes "mosona-manager/pkg/types"
	"mosona-manager/pkg/ws"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

const infoReportInterval = 10 * time.Minute

func nextInfoReportDelay() time.Duration {
	// Spread reports across a 20% window to avoid a reconnect/reporting herd.
	jitter := time.Duration(rand.Int64N(int64(infoReportInterval / 5)))
	return infoReportInterval - infoReportInterval/10 + jitter
}

func startInfoReporter() {
	go func() {
		for {
			time.Sleep(nextInfoReportDelay())
			if err := reportInfo(); err != nil {
				log.Println("Failed to periodically report info:", err)
			}
		}
	}()
}

func Run() {
	if err := config.LoadPrivateKey(); err != nil {
		log.Fatalln("Failed to load private key:", err)
	}

	for {
		if err := reportInfo(); err != nil {
			log.Println("Failed to report info:", err)
			time.Sleep(10 * time.Second)
			continue
		}
		break
	}
	startInfoReporter()

	var client *ws.Client
	for {
		var err error
		client, err = connectHub("/api/agent/ws")
		if err != nil {
			log.Println("Failed to connect to hub:", err)
			time.Sleep(10 * time.Second)
			continue
		}
		break
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
					log.Println("Failed to send status:", err)
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
				if !config.Current.NoTerminal {
					go terminal(string(msg.Data))
				}
			}
		}
	}
}
