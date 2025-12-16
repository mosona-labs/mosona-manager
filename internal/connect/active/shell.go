package active

import (
	"context"
	"crypto/ed25519"
	"encoding/pem"
	"errors"
	"mosona-manager/internal/db"
	secureWS "mosona-manager/pkg/securews"
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
	done := make(chan struct{})

	// Agent WS -> Client WS
	go func() {
		defer func() {
			once.Do(func() { close(done) })
		}()
		for {
			if wsConn == nil || client == nil {
				return
			}
			_, message, err := client.ReadMessage()
			if err != nil {
				return
			}
			data, err := sc.Decrypt(message)
			if err != nil {
				return
			}

			if err := wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
				break
			}
		}
	}()

	// Client WS -> Agent WS
	go func() {
		defer func() {
			once.Do(func() { close(done) })
		}()
		for {
			if wsConn == nil || client == nil {
				return
			}
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				return
			}
			data, err := sc.Encrypt(message)
			if err != nil {
				return
			}
			if err := client.SendMessage(websocket.TextMessage, data); err != nil {
				break
			}
		}
	}()

	<-done
	return nil
}
