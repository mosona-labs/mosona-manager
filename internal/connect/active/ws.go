package active

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mosona-manager/internal/runtime"
	"mosona-manager/pkg/identity"
	secureWS "mosona-manager/pkg/securews"
	pbTypes "mosona-manager/pkg/types"
	"mosona-manager/pkg/ws"
	"mosona-manager/pkg/wsutil"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

func (a *auth) connectAgent(ctx context.Context, path string, allowTOFU bool) (*ws.Client, *secureWS.SessionCrypto, error) {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, nil, err
	}
	httpNonce := base64.StdEncoding.EncodeToString(nonceBytes)
	httpTimestamp := strconv.FormatInt(time.Now().Unix(), 10)

	client := ws.NewClient()
	client.SetHeader("X-Agent-Id", a.agentUID)
	client.SetHeader("X-Agent-Timestamp", httpTimestamp)
	client.SetHeader("X-Agent-Nonce", httpNonce)
	ts, _ := strconv.ParseInt(httpTimestamp, 10, 64)
	signature, err := identity.SignHeaders(*a.privKey, a.agentUID, ts, httpNonce)
	if err != nil {
		return nil, nil, err
	}
	client.SetHeader("X-Agent-Signature", signature)
	client.SetHeader("User-Agent", "mosona-manager-hub/"+runtime.Version)
	client.SetReconnectConfig(0, 0)

	if err = client.Connect(ctx, fmt.Sprintf("ws://%s:%d%s", a.host, a.port, path)); err != nil {
		return nil, nil, err
	}
	if err = client.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	hctx := secureWS.HandshakeContext{AgentUID: a.agentUID, Path: path, HTTPNonce: httpNonce, HTTPTimestamp: httpTimestamp}
	sc, err := a.encryptConnection(client, hctx, allowTOFU)
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	if err = client.SetDeadline(time.Time{}); err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	pongWait := a.pongWait
	if pongWait <= 0 {
		pongWait = wsutil.DefaultPongWait
	}
	if err = client.SetPongDeadline(pongWait); err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return client, sc, nil
}

func (a *auth) connectAgentLegacy(ctx context.Context, path string) (*ws.Client, *secureWS.SessionCrypto, error) {
	if a.protocolVersion != 1 || len(a.agentPubKey) != 0 {
		return nil, nil, fmt.Errorf("legacy Active Agent handshake is not allowed after v2 pairing")
	}
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, nil, err
	}
	httpNonce := base64.StdEncoding.EncodeToString(nonceBytes)
	httpTimestamp := time.Now().Unix()
	client := ws.NewClient()
	client.SetHeader("X-Agent-Id", a.agentUID)
	client.SetHeader("X-Agent-Timestamp", strconv.FormatInt(httpTimestamp, 10))
	client.SetHeader("X-Agent-Nonce", httpNonce)
	signature, err := identity.SignHeaders(*a.privKey, a.agentUID, httpTimestamp, httpNonce)
	if err != nil {
		return nil, nil, err
	}
	client.SetHeader("X-Agent-Signature", signature)
	client.SetHeader("User-Agent", "mosona-manager-hub/"+runtime.Version)
	client.SetReconnectConfig(0, 0)
	if err = client.Connect(ctx, fmt.Sprintf("ws://%s:%d%s", a.host, a.port, path)); err != nil {
		return nil, nil, err
	}
	if err = client.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	sc, err := a.encryptConnectionLegacy(client)
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	if err = client.SetDeadline(time.Time{}); err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return client, sc, nil
}

func (a *auth) encryptConnectionLegacy(client *ws.Client) (*secureWS.SessionCrypto, error) {
	curve := ecdh.X25519()
	hubPrivate, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	hubNonceBytes := make([]byte, 16)
	if _, err = rand.Read(hubNonceBytes); err != nil {
		return nil, err
	}
	hubPublic := base64.StdEncoding.EncodeToString(hubPrivate.PublicKey().Bytes())
	hubNonce := base64.StdEncoding.EncodeToString(hubNonceBytes)
	timestamp := time.Now().Unix()
	signed := fmt.Sprintf("%s\n%d\n%s", hubPublic, timestamp, hubNonce)
	initMessage := pbTypes.KTHub{
		HubX25519Pub: hubPublic,
		HubNonce:     hubNonce,
		Timestamp:    timestamp,
		Sign:         base64.StdEncoding.EncodeToString(ed25519.Sign(*a.privKey, []byte(signed))),
	}
	encoded, err := msgpack.Marshal(initMessage)
	if err != nil {
		return nil, err
	}
	if err = client.SendMessage(websocket.BinaryMessage, encoded); err != nil {
		return nil, err
	}
	_, encoded, err = client.ReadMessage()
	if err != nil {
		return nil, err
	}
	var response pbTypes.KTAgent
	if err = msgpack.Unmarshal(encoded, &response); err != nil {
		return nil, err
	}
	if response.Version != 0 {
		return nil, fmt.Errorf("legacy Active Agent returned an unexpected protocol version")
	}
	agentPublicBytes, err := secureWS.DecodeExactBase64(response.AgentX25519Pub, 32)
	if err != nil {
		return nil, err
	}
	if _, err = secureWS.DecodeExactBase64(response.AgentNonce, 16); err != nil {
		return nil, err
	}
	agentPublic, err := curve.NewPublicKey(agentPublicBytes)
	if err != nil {
		return nil, err
	}
	return secureWS.NewSessionCrypto(secureWS.RoleHub, agentPublic, hubPrivate, hubNonce, response.AgentNonce)
}

func (a *auth) encryptConnection(client *ws.Client, hctx secureWS.HandshakeContext, allowTOFU bool) (*secureWS.SessionCrypto, error) {
	curve := ecdh.X25519()
	hubPriv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate X25519 keypair: %w", err)
	}
	hubNonce := make([]byte, 16)
	if _, err = rand.Read(hubNonce); err != nil {
		return nil, err
	}
	ts := time.Now().Unix()
	hubPub := hubPriv.PublicKey().Bytes()
	initMsg := pbTypes.KTHub{
		Version:      secureWS.ProtocolVersion,
		HubX25519Pub: base64.StdEncoding.EncodeToString(hubPub),
		HubNonce:     base64.StdEncoding.EncodeToString(hubNonce),
		Timestamp:    ts,
		Sign:         base64.StdEncoding.EncodeToString(ed25519.Sign(*a.privKey, secureWS.HubSignatureMessage(hctx, hubPub, hubNonce, ts))),
	}
	data, err := msgpack.Marshal(initMsg)
	if err != nil {
		return nil, err
	}
	if err = client.SendMessage(websocket.BinaryMessage, data); err != nil {
		return nil, err
	}

	_, responseData, err := client.ReadMessage()
	if err != nil {
		return nil, err
	}
	var response pbTypes.KTAgent
	if err = msgpack.Unmarshal(responseData, &response); err != nil {
		return nil, err
	}
	if response.Version != secureWS.ProtocolVersion {
		return nil, fmt.Errorf("active agent does not support authenticated handshake v2")
	}
	agentXPub, err := secureWS.DecodeExactBase64(response.AgentX25519Pub, 32)
	if err != nil {
		return nil, err
	}
	agentNonce, err := secureWS.DecodeExactBase64(response.AgentNonce, 16)
	if err != nil {
		return nil, err
	}
	agentIdentityBytes, err := secureWS.DecodeExactBase64(response.AgentEd25519Pub, ed25519.PublicKeySize)
	if err != nil {
		return nil, err
	}
	agentSignature, err := secureWS.DecodeExactBase64(response.Sign, ed25519.SignatureSize)
	if err != nil {
		return nil, err
	}

	transcriptHash := (secureWS.HandshakeTranscript{
		Context: hctx, HubX25519Pub: hubPub, HubNonce: hubNonce, HubTimestamp: ts,
		AgentEd25519Pub: agentIdentityBytes, AgentX25519Pub: agentXPub, AgentNonce: agentNonce,
	}).Hash()
	candidate := ed25519.PublicKey(agentIdentityBytes)
	if !ed25519.Verify(candidate, secureWS.AgentSignatureMessage(transcriptHash), agentSignature) {
		if candidatePEM, encodeErr := identity.EncodeEd25519PublicKeyPEM(candidate); encodeErr == nil {
			recordActiveIdentityEvent(a.serverID, candidatePEM, "Blocked invalid Active Agent identity proof", "high")
		}
		return nil, fmt.Errorf("active agent identity signature verification failed")
	}
	if len(a.agentPubKey) == 0 {
		if !allowTOFU {
			return nil, ErrAgentIdentityUnpaired
		}
		candidatePEM, err := identity.EncodeEd25519PublicKeyPEM(candidate)
		if err != nil {
			return nil, err
		}
		pinnedPEM, err := pinActiveAgentPublicKey(a.serverID, candidatePEM)
		if err != nil {
			return nil, err
		}
		a.agentPubKey, err = identity.ParseEd25519PublicKeyPEM([]byte(pinnedPEM))
		if err != nil {
			return nil, err
		}
		a.protocolVersion = 2
	}
	if !bytes.Equal(a.agentPubKey, candidate) {
		if candidatePEM, encodeErr := identity.EncodeEd25519PublicKeyPEM(candidate); encodeErr == nil {
			recordActiveIdentityEvent(a.serverID, candidatePEM, "Blocked changed Active Agent identity", "high")
		}
		return nil, ErrAgentIdentityMismatch
	}

	agentPub, err := curve.NewPublicKey(agentXPub)
	if err != nil {
		return nil, err
	}
	sc, err := secureWS.NewSessionCryptoV2(secureWS.RoleHub, agentPub, hubPriv, transcriptHash)
	if err != nil {
		return nil, err
	}
	if err = sendFinished(client, sc, []byte(secureWS.HubFinished)); err != nil {
		return nil, err
	}
	if err = receiveFinished(client, sc, []byte(secureWS.AgentFinished)); err != nil {
		return nil, err
	}
	return sc, nil
}

func sendFinished(client *ws.Client, sc *secureWS.SessionCrypto, value []byte) error {
	data, err := sc.Encrypt(value)
	if err != nil {
		return err
	}
	return client.SendMessage(websocket.BinaryMessage, data)
}

func receiveFinished(client *ws.Client, sc *secureWS.SessionCrypto, want []byte) error {
	_, data, err := client.ReadMessage()
	if err != nil {
		return err
	}
	plain, err := sc.Decrypt(data)
	if err != nil || !bytes.Equal(plain, want) {
		return fmt.Errorf("invalid active agent finished message")
	}
	return nil
}
