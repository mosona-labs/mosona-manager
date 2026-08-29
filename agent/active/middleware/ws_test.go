package middleware

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"mosona-manager/agent/config"
	secureWS "mosona-manager/pkg/securews"
	pbTypes "mosona-manager/pkg/types"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

func TestVerifyHandshakeInitRejectsInvalidConfiguredPublicKeyLength(t *testing.T) {
	oldPublicKey := config.PublicKey
	config.PublicKey = []byte("short")
	t.Cleanup(func() { config.PublicKey = oldPublicKey })

	message := &pbTypes.KTHub{
		Version:      secureWS.ProtocolVersion,
		HubX25519Pub: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		HubNonce:     base64.StdEncoding.EncodeToString(make([]byte, 16)),
		Timestamp:    time.Now().Unix(),
		Sign:         base64.StdEncoding.EncodeToString(make([]byte, 64)),
	}
	if err := verifyHandshakeInit(message, secureWS.HandshakeContext{}, time.Now()); err == nil || !strings.Contains(err.Error(), "public key length") {
		t.Fatalf("verifyHandshakeInit() error = %v, want public key length error", err)
	}
}

func TestVerifyLegacyHandshakeInitRejectsInvalidConfiguredPublicKeyLength(t *testing.T) {
	oldPublicKey := config.PublicKey
	config.PublicKey = []byte("short")
	t.Cleanup(func() { config.PublicKey = oldPublicKey })

	message := &pbTypes.KTHub{
		HubX25519Pub: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		HubNonce:     base64.StdEncoding.EncodeToString(make([]byte, 16)),
		Timestamp:    time.Now().Unix(),
		Sign:         base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)),
	}
	if err := verifyLegacyHandshakeInit(message, time.Now()); err == nil || !strings.Contains(err.Error(), "public key length") {
		t.Fatalf("verifyLegacyHandshakeInit() error = %v, want public key length error", err)
	}
}

func TestLegacyHandshakeInvalidConfiguredKeyClosesWebSocket(t *testing.T) {
	oldPublicKey := config.PublicKey
	config.PublicKey = []byte("short")
	t.Cleanup(func() { config.PublicKey = oldPublicKey })

	var called atomic.Bool
	server := httptest.NewServer(WsMiddleware(func(*websocket.Conn, *secureWS.SessionCrypto) {
		called.Store(true)
	}))
	t.Cleanup(server.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	message, err := msgpack.Marshal(pbTypes.KTHub{
		HubX25519Pub: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		HubNonce:     base64.StdEncoding.EncodeToString(make([]byte, 16)),
		Timestamp:    time.Now().Unix(),
		Sign:         base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = conn.WriteMessage(websocket.BinaryMessage, message); err != nil {
		t.Fatal(err)
	}
	if _, _, err = conn.ReadMessage(); err == nil {
		t.Fatal("invalid legacy handshake left the WebSocket open")
	}
	if called.Load() {
		t.Fatal("invalid legacy handshake invoked the business handler")
	}
}

func TestVerifyHandshakeInitBindsRequestContext(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	oldPublicKey := config.PublicKey
	config.PublicKey = publicKey
	t.Cleanup(func() { config.PublicKey = oldPublicKey })

	hctx := secureWS.HandshakeContext{AgentUID: "agent-1", Path: "/api/ws/terminal", HTTPNonce: "http-nonce", HTTPTimestamp: "123"}
	hubPub := make([]byte, 32)
	hubNonce := make([]byte, 16)
	now := time.Now()
	message := &pbTypes.KTHub{
		Version:      secureWS.ProtocolVersion,
		HubX25519Pub: base64.StdEncoding.EncodeToString(hubPub),
		HubNonce:     base64.StdEncoding.EncodeToString(hubNonce),
		Timestamp:    now.Unix(),
	}
	message.Sign = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, secureWS.HubSignatureMessage(hctx, hubPub, hubNonce, message.Timestamp)))
	if err = verifyHandshakeInit(message, hctx, now); err != nil {
		t.Fatal(err)
	}
	hctx.Path = "/api/ws/state"
	if err = verifyHandshakeInit(message, hctx, now); err == nil {
		t.Fatal("handshake signature accepted for a different path")
	}
}
