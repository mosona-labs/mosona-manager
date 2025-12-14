package aterminal

import (
	"mosona-manager/internal/app/agent/connection"
	pbTypes "mosona-manager/pkg/types"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

func terminalPassive(serverId int64, wsConn *websocket.Conn) error {
	id, _ := uuid.NewUUID()

	ma, ok := connection.MainGet(serverId)
	if !ok {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte("Agent connection not found. Please ensure the agent is online and terminal access is enabled.\n"))
		return wsConn.Close()
	}

	connection.UserSet(id.String(), wsConn)

	d, _ := msgpack.Marshal(pbTypes.Msg{
		Code: "terminal",
		Data: []byte(id.String()),
	})
	if err := ma.WriteMessage(websocket.BinaryMessage, d); err != nil {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte("Failed to request terminal session: "+err.Error()+"\n"))
		return wsConn.Close()
	}

	return nil
}
