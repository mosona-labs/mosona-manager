package aterminal

import (
	"context"
	"mosona-manager/internal/app/agent/connection"
	pbTypes "mosona-manager/pkg/types"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

func terminalPassive(ctx context.Context, serverId int64, wsConn *websocket.Conn) error {
	id, err := uuid.NewRandom()
	if err != nil {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte("Failed to create a secure terminal session.\n"))
		return wsConn.Close()
	}

	ma, ok := connection.MainGet(serverId)
	if !ok {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte("Agent connection not found. Please ensure the agent is online and terminal access is enabled.\n"))
		return wsConn.Close()
	}

	done, remove := connection.UserSet(id.String(), serverId, wsConn)
	defer remove()

	d, _ := msgpack.Marshal(pbTypes.Msg{
		Code: "terminal",
		Data: []byte(id.String()),
	})
	if err := ma.WriteMessage(websocket.BinaryMessage, d); err != nil {
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte("Failed to request terminal session: "+err.Error()+"\n"))
		return wsConn.Close()
	}
	select {
	case <-ctx.Done():
		remove()
		_ = wsConn.Close()
	case <-done:
	}
	return nil
}
