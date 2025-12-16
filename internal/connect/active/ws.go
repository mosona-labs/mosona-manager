package active

import (
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mosona-manager/agent/config"
	"mosona-manager/internal/runtime"
	"mosona-manager/pkg/identity"
	pbTypes "mosona-manager/pkg/types"
	"mosona-manager/pkg/ws"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

func (a *auth) connectAgent(ctx context.Context, path string) (*ws.Client, error) {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)
	ts := time.Now().Unix()

	client := ws.NewClient()
	client.SetHeader("X-Agent-Id", config.Current.UUID)
	client.SetHeader("X-Agent-Timestamp", fmt.Sprintf("%d", ts))
	client.SetHeader("X-Agent-Nonce", nonce)
	signature, err := identity.SignHeaders(config.PrivateKey, config.Current.UUID, ts, nonce)
	if err != nil {
		return nil, err
	}
	client.SetHeader("X-Agent-Signature", signature)

	client.SetHeader("User-Agent", "mosona-manager-hub/"+runtime.Version)

	client.SetReconnectConfig(-1, 10*time.Second)

	if err = client.Connect(ctx, fmt.Sprintf("%s:%d%s", a.host, a.port, path)); err != nil {
		return nil, err
	}

	// Encrypt connection
	if err = a.encryptConnection(client); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}

func (a *auth) encryptConnection(client *ws.Client) error {
	curve := ecdh.X25519()
	hubPrivKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate X25519 keypair: %w", err)
	}
	hubPub := hubPrivKey.PublicKey().Bytes()
	hubNonceBytes := make([]byte, 16)
	if _, err = rand.Read(hubNonceBytes); err != nil {
		return err
	}
	hubNonce := base64.StdEncoding.EncodeToString(hubNonceBytes)
	ts := time.Now().Unix()
	msg := fmt.Sprintf("%s\n%d\n%s",
		base64.StdEncoding.EncodeToString(hubPub),
		ts,
		hubNonce)
	sig := ed25519.Sign(*a.privKey, []byte(msg))

	// HandshakeInit
	initMsg := pbTypes.KTHub{
		HubX25519Pub: base64.StdEncoding.EncodeToString(hubPub),
		HubNonce:     hubNonce,
		Timestamp:    ts,
		Sign:         base64.StdEncoding.EncodeToString(sig),
	}
	data, err := msgpack.Marshal(initMsg)
	if err != nil {
		return err
	}
	if err = client.SendMessage(websocket.BinaryMessage, data); err != nil {
		return err
	}

	// Handshake response
	_, respData, err := client.ReadMessage()
	if err != nil {
		return err
	}
	var agentResp pbTypes.KTAgent
	if err = msgpack.Unmarshal(respData, &agentResp); err != nil {
		return err
	}

	// Agent Public Key (X25519)
	agentPubBytes, err := base64.StdEncoding.DecodeString(agentResp.AgentX25519Pub)
	if err != nil {
		return err
	}
	agentPubKey, err := curve.NewPublicKey(agentPubBytes)
	if err != nil {
		return err
	}

	a.xAgentPubKey = agentPubKey
	a.xHubPrivKey = hubPrivKey
	a.hubNonce = hubNonce
	a.agentNonce = agentResp.AgentNonce

	return nil
}
