package passive

import (
	"log"
	"mosona-manager/agent/config"
	"mosona-manager/agent/telemetry"
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

	client, err := connectHub()
	if err != nil {
		log.Fatalln("Failed to connect to hub:", err)
	}

	// Monitoring loop
	if !config.Current.NoMonitor {
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
	}

}
