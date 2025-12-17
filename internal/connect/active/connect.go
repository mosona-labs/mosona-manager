package active

import (
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"encoding/pem"
	"errors"
	"log"
	"mosona-manager/agent/types"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/influx"
	secureWS "mosona-manager/pkg/securews"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

type auth struct {
	serverID int64
	agentUID string
	host     string
	port     int
	privKey  *ed25519.PrivateKey

	// Encryption
	xAgentPubKey *ecdh.PublicKey
	xHubPrivKey  *ecdh.PrivateKey
	hubNonce     string
	agentNonce   string
}

func Connect(
	ctx context.Context,
	host string, port int,
	privKey string, agentUid string,
	serverId int64,
) error {
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

	if err := a.getInformation(); err != nil {
		return err
	}

	client, err := a.connectAgent(ctx, "/api/ws/state")
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

	// Read loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			_, originData, err := client.ReadMessage()
			if err != nil {
				return
			}
			data, err := sc.Decrypt(originData)
			if err != nil {
				continue
			}

			var state types.Status
			if err = msgpack.Unmarshal(data, &state); err != nil {
				continue
			}

			if err = influx.AddServerStatus(serverId, _type.ServerStatusType{
				CPU:           state.CPU,
				MemTotalMB:    state.MemTotalMB,
				MemUsedMB:     state.MemUsedMB,
				SwapTotalMB:   state.SwapTotalMB,
				SwapUsedMB:    state.SwapUsedMB,
				DiskTotalGB:   state.DiskTotalGB,
				DiskUsedGB:    state.DiskUsedGB,
				DiskReadKibS:  state.DiskReadKibS,
				DiskWriteKibS: state.DiskWriteKibS,
				DiskReadIOPS:  state.DiskReadIOPS,
				DiskWriteIOPS: state.DiskWriteIOPS,
				RxKibS:        state.RxKibS,
				TxKibS:        state.TxKibS,
				RxTotalMB:     state.RxTotalMB,
				TxTotalMB:     state.TxTotalMB,
				TCPTotal:      state.TCPTotal,
				UDPTotal:      state.UDPTotal,
				Time:          time.Now(),
			}); err != nil {
				log.Println("Failed to add server status:", err)
			}
		}
	}()

	// Ping
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
			if err := client.SendMessage(websocket.PingMessage, nil); err != nil {
				return err
			}
		}
	}
}
