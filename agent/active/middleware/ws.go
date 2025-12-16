package middleware

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mosona-manager/agent/config"
	pbTypes "mosona-manager/pkg/types"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

type HandlerWSFunc func(
	conn *websocket.Conn,
	xHubPubKey *ecdh.PublicKey,
	xAgentPrivKey *ecdh.PrivateKey,
	hubNonce string,
	agentNonce string,
)

func WsMiddleware(next HandlerWSFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "WebSocket upgrade failed", http.StatusBadRequest)
			return
		}

		// Error handler
		handleError := func(message string) {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(message))
			_ = conn.Close()
		}

		// HandshakeInit
		_, firstResp, err := conn.ReadMessage()
		if err != nil {
			handleError("Failed to read initial message")
			return
		}
		var initHub pbTypes.KTHub
		if err = msgpack.Unmarshal(firstResp, &initHub); err != nil {
			handleError("Failed to unmarshal initial message")
			return
		}
		now := time.Now()

		// Verify HandshakeInit
		if err = verifyHandshakeInit(&initHub, now); err != nil {
			handleError("Handshake verification failed")
			return
		}

		// Sign New Temp X25519 Keypair
		curve := ecdh.X25519()
		privKey, err := curve.GenerateKey(rand.Reader)
		if err != nil {
			handleError("Failed to generate X25519 keypair")
			return
		}
		pub := privKey.PublicKey().Bytes()

		// Send
		agentNonceBytes := make([]byte, 16)
		if _, err = rand.Read(agentNonceBytes); err != nil {
			handleError("Failed to generate agent nonce")
			return
		}
		agentNonce := base64.StdEncoding.EncodeToString(agentNonceBytes)
		data, _ := msgpack.Marshal(pbTypes.KTAgent{
			AgentX25519Pub: base64.StdEncoding.EncodeToString(pub),
			AgentNonce:     agentNonce,
		})
		if err = conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			handleError("Failed to send KTAgent message")
			return
		}

		// Convert
		hubPubKeyByte, _ := base64.StdEncoding.DecodeString(initHub.HubX25519Pub)
		hubPubKey, err := curve.NewPublicKey(hubPubKeyByte)
		if err != nil {
			handleError("Failed to parse hub X25519 public key")
			return
		}

		// Next handler
		next(conn, hubPubKey, privKey, initHub.HubNonce, agentNonce)
	}
}

func verifyHandshakeInit(initHub *pbTypes.KTHub, now time.Time) error {
	hubXPub, err := base64.StdEncoding.DecodeString(initHub.HubX25519Pub)
	if err != nil {
		return fmt.Errorf("invalid hub_x25519_pub base64: %w", err)
	}
	if len(hubXPub) != 32 {
		return fmt.Errorf("invalid hub_x25519_pub len: got=%d want=32", len(hubXPub))
	}
	nonce, err := base64.StdEncoding.DecodeString(initHub.HubNonce)
	if err != nil {
		return fmt.Errorf("invalid nonce encoding")
	}
	if len(nonce) < 16 {
		return fmt.Errorf("invalid nonce length")
	}
	sig, err := base64.StdEncoding.DecodeString(initHub.Sign)
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length")
	}
	const maxSkew = 120 * time.Second
	ts := time.Unix(initHub.Timestamp, 0)
	if ts.After(now.Add(maxSkew)) || ts.Before(now.Add(-maxSkew)) {
		return fmt.Errorf("timestamp out of range")
	}

	msg := fmt.Sprintf("%s\n%d\n%s", initHub.HubX25519Pub, initHub.Timestamp, initHub.HubNonce)
	if !ed25519.Verify(config.PublicKey, []byte(msg), sig) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}
