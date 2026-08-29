package active

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"mosona-manager/agent/types"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/influx"
	"mosona-manager/pkg/identity"
	secureWS "mosona-manager/pkg/securews"
	"mosona-manager/pkg/ws"
	"mosona-manager/pkg/wsutil"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

type auth struct {
	serverID        int64
	agentUID        string
	host            string
	port            int
	privKey         *ed25519.PrivateKey
	agentPubKey     ed25519.PublicKey
	protocolVersion int16
	pongWait        time.Duration
}

type stateMessageReader interface {
	ReadMessage() (messageType int, data []byte, err error)
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
	publicKey string, protocolVersion int16, serverId int64, allowMonitor bool,
) error {
	privateKey, err := identity.ParseEd25519PrivateKeyPEM([]byte(privKey))
	if err != nil {
		return err
	}

	a := &auth{
		serverID:        serverId,
		agentUID:        agentUid,
		host:            host,
		port:            port,
		privKey:         &privateKey,
		protocolVersion: protocolVersion,
	}
	if publicKey != "" {
		a.agentPubKey, err = identity.ParseEd25519PublicKeyPEM([]byte(publicKey))
		if err != nil {
			return err
		}
	}

	useLegacy := false
	if len(a.agentPubKey) == 0 {
		pairClient, _, pairErr := a.connectAgent(ctx, "/api/ws/pair", true)
		if pairErr != nil {
			if errors.Is(pairErr, ErrAgentIdentityMismatch) {
				return pairErr
			}
			if a.protocolVersion != 1 {
				return pairErr
			}
			useLegacy = true
		} else {
			_ = pairClient.Close()
			if err = markActiveAgentPaired(serverId); err != nil {
				return err
			}
		}
	}
	if !allowMonitor {
		if useLegacy {
			legacyClient, _, legacyErr := a.connectAgentLegacy(ctx, "/api/ws/state")
			if legacyErr != nil {
				return legacyErr
			}
			_ = legacyClient.Close()
			if err = markActiveAgentPaired(serverId); err != nil {
				return err
			}
			return ErrLegacyAgentRequiresUpgrade
		}
		return nil
	}
	if err = a.getInformation(ctx); err != nil {
		return err
	}

	var client *ws.Client
	var sc *secureWS.SessionCrypto
	if useLegacy {
		client, sc, err = a.connectAgentLegacy(ctx, "/api/ws/state")
	} else {
		client, sc, err = a.connectAgent(ctx, "/api/ws/state", false)
	}
	if err != nil {
		return err
	}

	// Read loop
	readDone := make(chan struct{})
	readErr := make(chan error, 1)
	go func() {
		defer close(readDone)
		readErr <- readActiveAgentStatuses(ctx, client, sc, serverId)
	}()
	defer func() {
		_ = client.Close()
		<-readDone
	}()

	var ping <-chan time.Time
	var pingTicker *time.Ticker
	if !useLegacy {
		pingTicker = time.NewTicker(wsutil.DefaultPingInterval)
		ping = pingTicker.C
		defer pingTicker.Stop()
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err = <-readErr:
			return err
		case <-ping:
			if err := client.SendMessage(websocket.PingMessage, nil); err != nil {
				return err
			}
		}
	}
}

func readActiveAgentStatuses(ctx context.Context, client stateMessageReader, sc *secureWS.SessionCrypto, serverID int64) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, originData, err := client.ReadMessage()
		if err != nil {
			return err
		}
		data, err := sc.Decrypt(originData)
		if err != nil {
			return fmt.Errorf("decrypt Active Agent status: %w", err)
		}

		var state types.Status
		if err = msgpack.Unmarshal(data, &state); err != nil {
			continue
		}

		if err = influx.AddServerStatusContext(ctx, serverID, _type.ServerStatusType{
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
}
