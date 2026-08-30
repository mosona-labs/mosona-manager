package active

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
	"mosona-manager/pkg/identity"
	secureWS "mosona-manager/pkg/securews"
	pbTypes "mosona-manager/pkg/types"
)

type fakeAgentMutation func(*pbTypes.KTAgent)

func TestEncryptConnectionAuthenticatesAgentResponse(t *testing.T) {
	_, expectedPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	expectedPublic := expectedPrivate.Public().(ed25519.PublicKey)

	tests := []struct {
		name          string
		identity      ed25519.PrivateKey
		mutate        fakeAgentMutation
		complete      bool
		finished      []byte
		wantError     string
		wantSentinel  error
		wantAuditRead bool
	}{
		{name: "valid v2", identity: expectedPrivate, complete: true},
		{
			name: "tampered signature", identity: expectedPrivate, wantError: "signature verification failed", wantAuditRead: true,
			mutate: func(response *pbTypes.KTAgent) {
				response.Sign = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x7f}, ed25519.SignatureSize))
			},
		},
		{
			name: "wrong identity key", identity: mustGenerateIdentity(t), wantSentinel: ErrAgentIdentityMismatch, wantAuditRead: true,
		},
		{
			name: "version zero downgrade", identity: expectedPrivate, wantError: "does not support authenticated handshake v2",
			mutate: func(response *pbTypes.KTAgent) {
				response.Version = 0
			},
		},
		{name: "invalid Agent Finished", identity: expectedPrivate, complete: true, finished: []byte("wrong finished"), wantError: "invalid active agent finished message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantAuditRead {
				mock := setupIdentityTest(t)
				expectIdentityAuditLookup(mock)
			}
			handlerDone := make(chan error, 1)
			host, port := startFakeAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgradeFakeAgent(w, r)
				if err == nil {
					_, err = serveFakeAgentHandshake(conn, r, tt.identity, tt.mutate, tt.complete, tt.finished)
				}
				handlerDone <- err
			})
			a := newTestHubAuth(t, host, port, expectedPublic)

			client, sc, err := a.connectAgent(context.Background(), "/api/ws/terminal", false)
			if client != nil {
				_ = client.Close()
			}
			waitFakeAgent(t, handlerDone)
			if tt.wantError == "" && tt.wantSentinel == nil {
				if err != nil || sc == nil {
					t.Fatalf("connectAgent() = (%v, %v), want authenticated session", sc, err)
				}
				return
			}
			if tt.wantSentinel != nil && !errors.Is(err, tt.wantSentinel) {
				t.Fatalf("connectAgent() error = %v, want %v", err, tt.wantSentinel)
			}
			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Fatalf("connectAgent() error = %v, want substring %q", err, tt.wantError)
			}
		})
	}
}

func TestEncryptConnectionRejectsReplayedKTAgent(t *testing.T) {
	identityPublic, identityPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	mock := setupIdentityTest(t)
	expectIdentityAuditLookup(mock)

	var mu sync.Mutex
	var response []byte
	connections := 0
	handlerDone := make(chan error, 2)
	host, port := startFakeAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgradeFakeAgent(w, r)
		if err != nil {
			handlerDone <- err
			return
		}
		mu.Lock()
		connections++
		current := connections
		replayed := append([]byte(nil), response...)
		mu.Unlock()
		if current == 1 {
			var encoded []byte
			encoded, err = serveFakeAgentHandshake(conn, r, identityPrivate, nil, true, nil)
			mu.Lock()
			response = append([]byte(nil), encoded...)
			mu.Unlock()
		} else {
			_, _, err = conn.ReadMessage()
			if err == nil {
				err = conn.WriteMessage(websocket.BinaryMessage, replayed)
			}
		}
		handlerDone <- err
	})
	a := newTestHubAuth(t, host, port, identityPublic)

	client, _, err := a.connectAgent(context.Background(), "/api/ws/terminal", false)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	waitFakeAgent(t, handlerDone)

	client, _, err = a.connectAgent(context.Background(), "/api/ws/terminal", false)
	if client != nil {
		_ = client.Close()
	}
	waitFakeAgent(t, handlerDone)
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("replayed KTAgent error = %v, want signature verification failure", err)
	}
}

func TestEncryptConnectionRejectsKeyChangeAfterTOFUPin(t *testing.T) {
	firstPublic, firstPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secondPrivate := mustGenerateIdentity(t)
	firstPEM, err := identity.EncodeEd25519PublicKeyPEM(firstPublic)
	if err != nil {
		t.Fatal(err)
	}
	mock := setupIdentityTest(t)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agents SET public_key = $1, protocol_version = 2 WHERE server_id = $2 AND public_key = '' AND EXISTS (SELECT 1 FROM servers WHERE id = $2 AND type = 1)")).
		WithArgs(firstPEM, int64(91)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectIdentityAuditLookup(mock)
	expectIdentityAuditLookup(mock)

	host, port, done := startSingleHandshakeAgent(t, firstPrivate, true)
	a := newTestHubAuth(t, host, port, nil)
	client, _, err := a.connectAgent(context.Background(), "/api/ws/pair", true)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	waitFakeAgent(t, done)
	if !bytes.Equal(a.agentPubKey, firstPublic) {
		t.Fatal("TOFU did not retain the first Agent identity")
	}

	host, port, done = startSingleHandshakeAgent(t, secondPrivate, false)
	a.host, a.port = host, port
	client, _, err = a.connectAgent(context.Background(), "/api/ws/terminal", false)
	if client != nil {
		_ = client.Close()
	}
	waitFakeAgent(t, done)
	if !errors.Is(err, ErrAgentIdentityMismatch) {
		t.Fatalf("changed identity error = %v, want ErrAgentIdentityMismatch", err)
	}
}

func TestConnectKeepsPreUpgradeAgentOnlineWithLegacyFallback(t *testing.T) {
	privateKeyPEM, _, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	mock := setupIdentityTest(t)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agents SET status = 1, last_seen_at = NOW() WHERE server_id = $1")).
		WithArgs(int64(91)).WillReturnResult(sqlmock.NewResult(0, 1))
	stateDone := make(chan error, 1)
	host, port := startFakeAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ws/pair" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/api/ws/state" {
			http.NotFound(w, r)
			return
		}
		conn, err := upgradeFakeAgent(w, r)
		if err == nil {
			err = serveFakeLegacyAgent(conn)
		}
		stateDone <- err
	})

	if err = Connect(context.Background(), host, port, privateKeyPEM, "agent-uid", "", 1, 91, false); !errors.Is(err, ErrLegacyAgentRequiresUpgrade) {
		t.Fatalf("Connect() error = %v, want legacy upgrade retry", err)
	}
	waitFakeAgent(t, stateDone)
}

func TestLegacyWriteOnlyAgentOutlivesV2PongWindow(t *testing.T) {
	done := make(chan error, 1)
	host, port := startFakeAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgradeFakeAgent(w, r)
		if err == nil {
			err = serveDelayedLegacyStatus(conn, 150*time.Millisecond, []byte("legacy status"))
		}
		done <- err
	})
	a := newTestHubAuth(t, host, port, nil)
	a.protocolVersion = 1
	a.pongWait = 50 * time.Millisecond

	client, sc, err := a.connectAgentLegacy(context.Background(), "/api/ws/state")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	_, frame, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("legacy connection closed after the v2 pong window: %v", err)
	}
	plain, err := sc.Decrypt(frame)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "legacy status" {
		t.Fatalf("legacy status = %q", plain)
	}
	waitFakeAgent(t, done)
}

func TestConnectNeverFallsBackAfterProtocolV2IsRequired(t *testing.T) {
	privateKeyPEM, _, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	var legacyStateRequests atomic.Int32
	host, port := startFakeAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ws/state" {
			legacyStateRequests.Add(1)
		}
		http.NotFound(w, r)
	})

	err = Connect(context.Background(), host, port, privateKeyPEM, "agent-uid", "", 2, 91, false)
	if err == nil {
		t.Fatal("protocol v2 record fell back after pairing failed")
	}
	if got := legacyStateRequests.Load(); got != 0 {
		t.Fatalf("legacy state requests = %d, want 0", got)
	}
}

func TestConnectNeverFallsBackAfterConcurrentTOFUMismatch(t *testing.T) {
	privateKeyPEM, _, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	candidatePrivate := mustGenerateIdentity(t)
	winnerPEM := testPublicKeyPEM(t)
	mock := setupIdentityTest(t)
	mock.ExpectExec("UPDATE agents SET public_key").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT public_key FROM agents WHERE server_id = $1")).
		WithArgs(int64(91)).WillReturnRows(sqlmock.NewRows([]string{"public_key"}).AddRow(winnerPEM))

	var legacyStateRequests atomic.Int32
	done := make(chan error, 1)
	host, port := startFakeAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ws/pair":
			conn, upgradeErr := upgradeFakeAgent(w, r)
			if upgradeErr == nil {
				_, upgradeErr = serveFakeAgentHandshake(conn, r, candidatePrivate, nil, false, nil)
			}
			done <- upgradeErr
		case "/api/ws/state":
			legacyStateRequests.Add(1)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	err = Connect(context.Background(), host, port, privateKeyPEM, "agent-uid", "", 1, 91, false)
	waitFakeAgent(t, done)
	if !errors.Is(err, ErrAgentIdentityMismatch) {
		t.Fatalf("Connect() error = %v, want ErrAgentIdentityMismatch", err)
	}
	if got := legacyStateRequests.Load(); got != 0 {
		t.Fatalf("legacy state requests = %d, want 0", got)
	}
}

func TestConnectShellNeverFallsBackAfterConcurrentTOFUMismatch(t *testing.T) {
	privateKeyPEM, _, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	candidatePrivate := mustGenerateIdentity(t)
	winnerPEM := testPublicKeyPEM(t)
	var legacyTerminalRequests atomic.Int32
	done := make(chan error, 1)
	host, port := startFakeAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ws/pair":
			conn, upgradeErr := upgradeFakeAgent(w, r)
			if upgradeErr == nil {
				_, upgradeErr = serveFakeAgentHandshake(conn, r, candidatePrivate, nil, false, nil)
			}
			done <- upgradeErr
		case "/api/ws/terminal":
			legacyTerminalRequests.Add(1)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	mock := setupIdentityTest(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT private_key, agent_uid, host, port, public_key, protocol_version FROM agents WHERE server_id = $1")).
		WithArgs(int64(91)).WillReturnRows(sqlmock.NewRows([]string{"private_key", "agent_uid", "host", "port", "public_key", "protocol_version"}).
		AddRow(testEncryptedAgentPrivateKey(t, 91, privateKeyPEM), "agent-uid", host, port, "", 1))
	mock.ExpectExec("UPDATE agents SET public_key").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT public_key FROM agents WHERE server_id = $1")).
		WithArgs(int64(91)).WillReturnRows(sqlmock.NewRows([]string{"public_key"}).AddRow(winnerPEM))

	err = ConnectShell(context.Background(), 91, nil)
	waitFakeAgent(t, done)
	if !errors.Is(err, ErrAgentIdentityMismatch) {
		t.Fatalf("ConnectShell() error = %v, want ErrAgentIdentityMismatch", err)
	}
	if got := legacyTerminalRequests.Load(); got != 0 {
		t.Fatalf("legacy terminal requests = %d, want 0", got)
	}
}

func TestConnectShellRejectsUnprotectedOrSwappedPrivateKey(t *testing.T) {
	tests := []struct {
		name       string
		privateKey func(*testing.T) []byte
	}{
		{name: "plaintext", privateKey: func(*testing.T) []byte { return []byte("plaintext-private-key") }},
		{name: "different server context", privateKey: func(t *testing.T) []byte {
			return testEncryptedAgentPrivateKey(t, 92, "private-key")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := setupIdentityTest(t)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT private_key, agent_uid, host, port, public_key, protocol_version FROM agents WHERE server_id = $1")).
				WithArgs(int64(91)).
				WillReturnRows(sqlmock.NewRows([]string{"private_key", "agent_uid", "host", "port", "public_key", "protocol_version"}).
					AddRow(tt.privateKey(t), "agent-uid", "127.0.0.1", 10000, "", 2))

			if err := ConnectShell(context.Background(), 91, nil); err == nil {
				t.Fatal("ConnectShell accepted an unprotected or record-swapped Agent private key")
			}
		})
	}
}

func serveFakeAgentHandshake(conn *websocket.Conn, r *http.Request, identityPrivate ed25519.PrivateKey, mutate fakeAgentMutation, complete bool, agentFinished []byte) ([]byte, error) {
	defer func() { _ = conn.Close() }()
	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var initHub pbTypes.KTHub
	if err = msgpack.Unmarshal(data, &initHub); err != nil {
		return nil, err
	}
	hubPubBytes, err := base64.StdEncoding.DecodeString(initHub.HubX25519Pub)
	if err != nil {
		return nil, err
	}
	hubNonce, err := base64.StdEncoding.DecodeString(initHub.HubNonce)
	if err != nil {
		return nil, err
	}
	curve := ecdh.X25519()
	agentXPrivate, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	agentNonce := make([]byte, 16)
	if _, err = rand.Read(agentNonce); err != nil {
		return nil, err
	}
	identityPublic := identityPrivate.Public().(ed25519.PublicKey)
	hctx := secureWS.HandshakeContext{
		AgentUID: "agent-uid", Path: r.URL.Path,
		HTTPNonce: r.Header.Get("X-Agent-Nonce"), HTTPTimestamp: r.Header.Get("X-Agent-Timestamp"),
	}
	transcriptHash := (secureWS.HandshakeTranscript{
		Context: hctx, HubX25519Pub: hubPubBytes, HubNonce: hubNonce, HubTimestamp: initHub.Timestamp,
		AgentEd25519Pub: identityPublic, AgentX25519Pub: agentXPrivate.PublicKey().Bytes(), AgentNonce: agentNonce,
	}).Hash()
	response := pbTypes.KTAgent{
		Version:         secureWS.ProtocolVersion,
		AgentX25519Pub:  base64.StdEncoding.EncodeToString(agentXPrivate.PublicKey().Bytes()),
		AgentNonce:      base64.StdEncoding.EncodeToString(agentNonce),
		AgentEd25519Pub: base64.StdEncoding.EncodeToString(identityPublic),
		Sign:            base64.StdEncoding.EncodeToString(ed25519.Sign(identityPrivate, secureWS.AgentSignatureMessage(transcriptHash))),
	}
	if mutate != nil {
		mutate(&response)
	}
	encoded, err := msgpack.Marshal(response)
	if err != nil {
		return nil, err
	}
	if err = conn.WriteMessage(websocket.BinaryMessage, encoded); err != nil || !complete {
		return encoded, err
	}
	hubPublic, err := curve.NewPublicKey(hubPubBytes)
	if err != nil {
		return encoded, err
	}
	sc, err := secureWS.NewSessionCryptoV2(secureWS.RoleAgent, hubPublic, agentXPrivate, transcriptHash)
	if err != nil {
		return encoded, err
	}
	_, frame, err := conn.ReadMessage()
	if err != nil {
		return encoded, err
	}
	finished, err := sc.Decrypt(frame)
	if err != nil || !bytes.Equal(finished, []byte(secureWS.HubFinished)) {
		return encoded, fmt.Errorf("invalid Hub Finished: %w", err)
	}
	if agentFinished == nil {
		agentFinished = []byte(secureWS.AgentFinished)
	}
	frame, err = sc.Encrypt(agentFinished)
	if err == nil {
		err = conn.WriteMessage(websocket.BinaryMessage, frame)
	}
	return encoded, err
}

func serveFakeLegacyAgent(conn *websocket.Conn) error {
	defer func() { _ = conn.Close() }()
	_, encoded, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	var initHub pbTypes.KTHub
	if err = msgpack.Unmarshal(encoded, &initHub); err != nil {
		return err
	}
	if initHub.Version != 0 {
		return fmt.Errorf("legacy Agent received protocol version %d", initHub.Version)
	}
	curve := ecdh.X25519()
	privateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	response := pbTypes.KTAgent{
		AgentX25519Pub: base64.StdEncoding.EncodeToString(privateKey.PublicKey().Bytes()),
		AgentNonce:     base64.StdEncoding.EncodeToString(nonce),
	}
	encoded, err = msgpack.Marshal(response)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, encoded)
}

func serveDelayedLegacyStatus(conn *websocket.Conn, delay time.Duration, status []byte) error {
	defer func() { _ = conn.Close() }()
	_, encoded, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	var initHub pbTypes.KTHub
	if err = msgpack.Unmarshal(encoded, &initHub); err != nil {
		return err
	}
	hubPublicBytes, err := base64.StdEncoding.DecodeString(initHub.HubX25519Pub)
	if err != nil {
		return err
	}
	curve := ecdh.X25519()
	privateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	response := pbTypes.KTAgent{
		AgentX25519Pub: base64.StdEncoding.EncodeToString(privateKey.PublicKey().Bytes()),
		AgentNonce:     base64.StdEncoding.EncodeToString(nonce),
	}
	encoded, err = msgpack.Marshal(response)
	if err != nil {
		return err
	}
	if err = conn.WriteMessage(websocket.BinaryMessage, encoded); err != nil {
		return err
	}
	hubPublic, err := curve.NewPublicKey(hubPublicBytes)
	if err != nil {
		return err
	}
	sc, err := secureWS.NewSessionCrypto(secureWS.RoleAgent, hubPublic, privateKey, initHub.HubNonce, response.AgentNonce)
	if err != nil {
		return err
	}
	time.Sleep(delay)
	frame, err := sc.Encrypt(status)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, frame)
}

func startSingleHandshakeAgent(t *testing.T, identityPrivate ed25519.PrivateKey, complete bool) (string, int, <-chan error) {
	t.Helper()
	done := make(chan error, 1)
	host, port := startFakeAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgradeFakeAgent(w, r)
		if err == nil {
			_, err = serveFakeAgentHandshake(conn, r, identityPrivate, nil, complete, nil)
		}
		done <- err
	})
	return host, port, done
}

func startFakeAgentServer(t *testing.T, handler http.HandlerFunc) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	host, portString, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func upgradeFakeAgent(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
}

func newTestHubAuth(t *testing.T, host string, port int, agentPublic ed25519.PublicKey) *auth {
	t.Helper()
	_, hubPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &auth{serverID: 91, agentUID: "agent-uid", host: host, port: port, privKey: &hubPrivate, agentPubKey: agentPublic}
}

func mustGenerateIdentity(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return privateKey
}

func expectIdentityAuditLookup(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT team_id FROM servers WHERE id = $1")).
		WithArgs(int64(91)).WillReturnRows(sqlmock.NewRows([]string{"team_id"}).AddRow(42))
}

func waitFakeAgent(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fake Agent handler did not finish")
	}
}
