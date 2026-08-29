package active

import (
	secureWS "mosona-manager/pkg/securews"

	"github.com/gorilla/websocket"
)

func handlePairWebSocket(conn *websocket.Conn, _ *secureWS.SessionCrypto) {
	_ = conn.Close()
}
