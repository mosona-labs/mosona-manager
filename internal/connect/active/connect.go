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

func agentDisksToServerDisks(agentDisks []types.DiskInfo) []_type.DiskInfo {
	if len(agentDisks) == 0 {
		return nil
	}
	disks := make([]_type.DiskInfo, len(agentDisks))
	for i, d := range agentDisks {
		disks[i] = _type.DiskInfo{
			MountPoint: d.MountPoint,
			TotalGB:    d.TotalGB,
			UsedGB:     d.UsedGB,
		}
	}
	return disks
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

	if err := a.getInformation(ctx); err != nil {
		return err
	}

	client, err := a.connectAgent(ctx, "/api/ws/state")
	if err != nil {
		return err
	}
	sc, err := secureWS.NewSessionCrypto(
		secureWS.RoleHub, a.xAgentPubKey, a.xHubPrivKey, a.hubNonce, a.agentNonce,
	)
	if err != nil {
		_ = client.Close()
		return err
	}

	// Read loop
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
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

			if err = influx.AddServerStatusContext(ctx, serverId, _type.ServerStatusType{
				CPU:           state.CPU,
				MemTotalMB:    state.MemTotalMB,
				MemUsedMB:     state.MemUsedMB,
				SwapTotalMB:   state.SwapTotalMB,
				SwapUsedMB:    state.SwapUsedMB,
				Disks:         agentDisksToServerDisks(state.Disks),
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
	defer func() {
		_ = client.Close()
		<-readDone
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
