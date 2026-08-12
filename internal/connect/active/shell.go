package active

import (
	"context"
	"crypto/ed25519"
	"encoding/pem"
	"errors"
	"mosona-manager/internal/db"
	secureWS "mosona-manager/pkg/securews"
	"mosona-manager/pkg/wsutil"
	"sync"

	"github.com/gorilla/websocket"
)

func ConnectShell(ctx context.Context, serverId int64, wsConn *websocket.Conn) error {
	var (
		privKey,
		agentUid,
		host string
		port int
	)
	if err := db.Db.QueryRow("SELECT private_key, agent_uid, host, port FROM agents WHERE server_id = $1", serverId).Scan(
		&privKey,
		&agentUid,
		&host,
		&port,
	); err != nil {
		return err
	}

	block, _ := pem.Decode([]byte(privKey))
	if block == nil {
		return errors.New("failed to decode PEM block")
	}
	privateKey := ed25519.NewKeyFromSeed(block.Bytes)
	a := &auth{
		serverID: serverId,
		agentUID: agentUid,
		host:     host,
		port:     port,
		privKey:  &privateKey,
	}
	client, err := a.connectAgent(ctx, "/api/ws/terminal")
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()

	sc, err := secureWS.NewSessionCrypto(
		secureWS.RoleHub, a.xAgentPubKey, a.xHubPrivKey, a.hubNonce, a.agentNonce,
	)
	if err != nil {
		return err
	}

	var once sync.Once
	var wsWriteMu sync.Mutex
	done := make(chan struct{})

	cleanup := func() {
		once.Do(func() {
			close(done)
			_ = client.Close()
			_ = wsConn.Close()
		})
	}
	wsutil.StartPing(ctx, wsConn, &wsWriteMu, "active terminal browser websocket ping")
	go func() {
		<-ctx.Done()
		cleanup()
	}()

	// Agent WS -> Client WS
	go func() {
		defer cleanup()
		for {
			_, message, err := client.ReadMessage()
			if err != nil {
				return
			}
			data, err := sc.Decrypt(message)
			if err != nil {
				return
			}

			wsWriteMu.Lock()
			err = wsConn.WriteMessage(websocket.BinaryMessage, data)
			wsWriteMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	// Client WS -> Agent WS
	go func() {
		defer cleanup()
		for {
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				return
			}
			data, err := sc.Encrypt(message)
			if err != nil {
				return
			}
			if err := client.SendMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}()

	<-done
	return nil
}
