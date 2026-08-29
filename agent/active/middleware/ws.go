package middleware

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mosona-manager/agent/config"
	"mosona-manager/pkg/httporigin"
	secureWS "mosona-manager/pkg/securews"
	pbTypes "mosona-manager/pkg/types"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

const handshakeTimeout = 15 * time.Second

type HandlerWSFunc func(*websocket.Conn, *secureWS.SessionCrypto)

func WsMiddleware(next HandlerWSFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{CheckOrigin: httporigin.SameOrigin}).Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "WebSocket upgrade failed", http.StatusBadRequest)
			return
		}
		defer func() { _ = conn.Close() }()
		handleError := func(message string) {
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, message))
		}
		_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
		_ = conn.SetWriteDeadline(time.Now().Add(handshakeTimeout))

		_, first, err := conn.ReadMessage()
		if err != nil {
			handleError("handshake read failed")
			return
		}
		var initHub pbTypes.KTHub
		if err = msgpack.Unmarshal(first, &initHub); err != nil {
			handleError("invalid handshake")
			return
		}
		hctx := secureWS.HandshakeContext{
			AgentUID:      config.Current.Uid,
			Path:          r.URL.Path,
			HTTPNonce:     r.Header.Get("X-Agent-Nonce"),
			HTTPTimestamp: r.Header.Get("X-Agent-Timestamp"),
		}
		if initHub.Version == 0 {
			// TODO: v1 Hub handshake; remove once protocol_version=1 is gone.
			sc, err := legacyHandshake(conn, &initHub, time.Now())
			if err != nil {
				handleError("handshake verification failed")
				return
			}
			clearDeadlines(conn)
			next(conn, sc)
			return
		}
		if err = verifyHandshakeInit(&initHub, hctx, time.Now()); err != nil {
			handleError("handshake verification failed")
			return
		}

		curve := ecdh.X25519()
		xPriv, err := curve.GenerateKey(rand.Reader)
		if err != nil {
			handleError("handshake key generation failed")
			return
		}
		agentNonce := make([]byte, 16)
		if _, err = rand.Read(agentNonce); err != nil {
			handleError("handshake nonce generation failed")
			return
		}
		hubPubBytes, _ := secureWS.DecodeExactBase64(initHub.HubX25519Pub, 32)
		hubNonce, _ := secureWS.DecodeExactBase64(initHub.HubNonce, 16)
		agentIdentity := config.PrivateKey.Public().(ed25519.PublicKey)
		transcript := secureWS.HandshakeTranscript{
			Context:         hctx,
			HubX25519Pub:    hubPubBytes,
			HubNonce:        hubNonce,
			HubTimestamp:    initHub.Timestamp,
			AgentEd25519Pub: agentIdentity,
			AgentX25519Pub:  xPriv.PublicKey().Bytes(),
			AgentNonce:      agentNonce,
		}
		transcriptHash := transcript.Hash()
		resp := pbTypes.KTAgent{
			Version:         secureWS.ProtocolVersion,
			AgentX25519Pub:  base64.StdEncoding.EncodeToString(xPriv.PublicKey().Bytes()),
			AgentNonce:      base64.StdEncoding.EncodeToString(agentNonce),
			AgentEd25519Pub: base64.StdEncoding.EncodeToString(agentIdentity),
			Sign:            base64.StdEncoding.EncodeToString(ed25519.Sign(config.PrivateKey, secureWS.AgentSignatureMessage(transcriptHash))),
		}
		data, err := msgpack.Marshal(resp)
		if err != nil || conn.WriteMessage(websocket.BinaryMessage, data) != nil {
			handleError("handshake response failed")
			return
		}
		hubPub, err := curve.NewPublicKey(hubPubBytes)
		if err != nil {
			handleError("invalid hub key")
			return
		}
		sc, err := secureWS.NewSessionCryptoV2(secureWS.RoleAgent, hubPub, xPriv, transcriptHash)
		if err != nil || receiveFinished(conn, sc, []byte(secureWS.HubFinished)) != nil || sendFinished(conn, sc, []byte(secureWS.AgentFinished)) != nil {
			handleError("handshake confirmation failed")
			return
		}
		clearDeadlines(conn)
		next(conn, sc)
	}
}

func verifyHandshakeInit(initHub *pbTypes.KTHub, hctx secureWS.HandshakeContext, now time.Time) error {
	if initHub.Version != secureWS.ProtocolVersion {
		return fmt.Errorf("invalid protocol version")
	}
	if len(config.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid configured public key length")
	}
	hubPub, err := secureWS.DecodeExactBase64(initHub.HubX25519Pub, 32)
	if err != nil {
		return err
	}
	hubNonce, err := secureWS.DecodeExactBase64(initHub.HubNonce, 16)
	if err != nil {
		return err
	}
	sig, err := secureWS.DecodeExactBase64(initHub.Sign, ed25519.SignatureSize)
	if err != nil {
		return err
	}
	if ts := time.Unix(initHub.Timestamp, 0); ts.After(now.Add(2*time.Minute)) || ts.Before(now.Add(-2*time.Minute)) {
		return fmt.Errorf("timestamp out of range")
	}
	if !ed25519.Verify(config.PublicKey, secureWS.HubSignatureMessage(hctx, hubPub, hubNonce, initHub.Timestamp), sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func legacyHandshake(conn *websocket.Conn, initHub *pbTypes.KTHub, now time.Time) (*secureWS.SessionCrypto, error) {
	if err := verifyLegacyHandshakeInit(initHub, now); err != nil {
		return nil, err
	}
	curve := ecdh.X25519()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	resp := pbTypes.KTAgent{AgentX25519Pub: base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes()), AgentNonce: base64.StdEncoding.EncodeToString(nonce)}
	data, err := msgpack.Marshal(resp)
	if err != nil {
		return nil, err
	}
	if err = conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return nil, err
	}
	hubBytes, _ := base64.StdEncoding.DecodeString(initHub.HubX25519Pub)
	hubPub, err := curve.NewPublicKey(hubBytes)
	if err != nil {
		return nil, err
	}
	return secureWS.NewSessionCrypto(secureWS.RoleAgent, hubPub, priv, initHub.HubNonce, resp.AgentNonce)
}

func verifyLegacyHandshakeInit(initHub *pbTypes.KTHub, now time.Time) error {
	if len(config.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid configured public key length")
	}
	_, err := secureWS.DecodeExactBase64(initHub.HubX25519Pub, 32)
	if err != nil {
		return err
	}
	if _, err = secureWS.DecodeExactBase64(initHub.HubNonce, 16); err != nil {
		return err
	}
	sig, err := secureWS.DecodeExactBase64(initHub.Sign, ed25519.SignatureSize)
	if err != nil {
		return err
	}
	if ts := time.Unix(initHub.Timestamp, 0); ts.After(now.Add(2*time.Minute)) || ts.Before(now.Add(-2*time.Minute)) {
		return fmt.Errorf("timestamp out of range")
	}
	msg := fmt.Sprintf("%s\n%d\n%s", initHub.HubX25519Pub, initHub.Timestamp, initHub.HubNonce)
	if !ed25519.Verify(config.PublicKey, []byte(msg), sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func receiveFinished(conn *websocket.Conn, sc *secureWS.SessionCrypto, want []byte) error {
	_, data, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	plain, err := sc.Decrypt(data)
	if err != nil || !bytes.Equal(plain, want) {
		return fmt.Errorf("invalid finished message")
	}
	return nil
}

func sendFinished(conn *websocket.Conn, sc *secureWS.SessionCrypto, value []byte) error {
	data, err := sc.Encrypt(value)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, data)
}

func clearDeadlines(conn *websocket.Conn) {
	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Time{})
}
