package active

import (
	"crypto/ecdh"
	"log"
	"mosona-manager/agent/config"
	"mosona-manager/agent/telemetry"
	"mosona-manager/pkg/securews"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

func handleStateWebSocket(
	conn *websocket.Conn,
	xHubPubKey *ecdh.PublicKey,
	xAgentPrivKey *ecdh.PrivateKey,
	hubNonce string,
	agentNonce string,
) {
	defer func() {
		_ = conn.Close()
	}()

	sc, err := secureWS.NewSessionCrypto(secureWS.RoleAgent, xHubPubKey, xAgentPrivKey, hubNonce, agentNonce)
	if err != nil {
		_ = conn.WriteMessage(websocket.CloseMessage, []byte("crypto init failed"))
		return
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

			if frame, err := sc.Encrypt(data); err == nil {
				if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
					return
				}
			}

			sleepFor := 3*time.Second - time.Since(start)
			if sleepFor > 0 {
				time.Sleep(sleepFor)
			}
		}
	}
}
