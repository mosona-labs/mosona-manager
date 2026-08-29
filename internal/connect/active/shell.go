package active

import (
	"context"
	"crypto/ed25519"
	"errors"
	"mosona-manager/internal/db"
	"mosona-manager/pkg/identity"
	secureWS "mosona-manager/pkg/securews"
	"mosona-manager/pkg/ws"
	"mosona-manager/pkg/wsutil"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

func ConnectShell(ctx context.Context, serverId int64, wsConn *websocket.Conn) error {
	var (
		privKey,
		agentUid,
		host,
		publicKey string
		port            int
		protocolVersion int16
	)
	if err := db.Db.QueryRow("SELECT private_key, agent_uid, host, port, public_key, protocol_version FROM agents WHERE server_id = $1", serverId).Scan(
		&privKey,
		&agentUid,
		&host,
		&port,
		&publicKey,
		&protocolVersion,
	); err != nil {
		return err
	}

	privateKey, err := identity.ParseEd25519PrivateKeyPEM([]byte(privKey))
	if err != nil {
		return err
	}
	var agentPublicKey ed25519.PublicKey
	if publicKey != "" {
		agentPublicKey, err = identity.ParseEd25519PublicKeyPEM([]byte(publicKey))
		if err != nil {
			return err
		}
	}
	a := &auth{
		serverID:        serverId,
		agentUID:        agentUid,
		host:            host,
		port:            port,
		privKey:         &privateKey,
		agentPubKey:     agentPublicKey,
		protocolVersion: protocolVersion,
	}
	var client *ws.Client
	var sc *secureWS.SessionCrypto
	useLegacy := false
	if len(a.agentPubKey) == 0 {
		pairClient, _, pairErr := a.connectAgent(ctx, "/api/ws/pair", true)
		if pairErr == nil {
			_ = pairClient.Close()
			if err = markActiveAgentPaired(serverId); err != nil {
				return err
			}
		} else if errors.Is(pairErr, ErrAgentIdentityMismatch) || a.protocolVersion != 1 {
			return pairErr
		} else {
			client, sc, err = a.connectAgentLegacy(ctx, "/api/ws/terminal")
			useLegacy = true
		}
	}
	if client == nil && err == nil {
		client, sc, err = a.connectAgent(ctx, "/api/ws/terminal", false)
	}
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()

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
	if !useLegacy {
		go func() {
			ticker := time.NewTicker(wsutil.DefaultPingInterval)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					if err := client.SendMessage(websocket.PingMessage, nil); err != nil {
						cleanup()
						return
					}
				}
			}
		}()
	}
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
