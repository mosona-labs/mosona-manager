package ateam

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/notification"
	"mosona-manager/internal/security/exportcrypto"
	"mosona-manager/internal/utils/encrypt"
	"mosona-manager/pkg/identity"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
	"github.com/lib/pq"
	gossh "golang.org/x/crypto/ssh"
)

func TestLegacySSHHostKeyConfirmationRequiredResponse(t *testing.T) {
	e := echo.New()
	e.GET("/confirm", func(c *echo.Context) error {
		return legacySSHHostKeyConfirmationRequired(c, []string{"legacy-a", "legacy-b"})
	})
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/confirm", nil))

	body := recorder.Body.String()
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, body)
	}
	for _, fragment := range []string{
		`"code":"legacy_ssh_host_key_confirmation_required"`,
		`"count":2`,
		`"servers":["legacy-a","legacy-b"]`,
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("response does not contain %q: %s", fragment, body)
		}
	}
}

func TestImportNotificationsRejectsUnsafeTargetBeforeDatabaseWrite(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	mock.ExpectBegin()
	tx, err := sqlx.NewDb(database, "sqlmock").Beginx()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectRollback()

	err = importNotifications(tx, 5, []_type.TeamNotification{{
		Module: "shoutrrr", Target: "unknown://example.com/hook",
	}})
	if !errors.Is(err, notification.ErrInvalidConfiguration) {
		t.Fatalf("error = %v", err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTeamImportRejectsUnsafeNotificationBeforeTransaction(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() { db.Db = oldDB })

	_, _, err = applyTeamImport(5, teamExportBundle{
		Notifications: []_type.TeamNotification{{
			Module: "shoutrrr", Target: "unknown://example.com/hook",
		}},
	}, false)
	if !errors.Is(err, notification.ErrInvalidConfiguration) {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTeamImportRejectsUnpinnedSSHBeforeTransaction(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() { db.Db = oldDB })

	_, _, err = applyTeamImport(5, teamExportBundle{Servers: []teamExportServer{{
		Type: 0,
		Name: "legacy",
		SSH:  &teamExportSSH{Address: "legacy.example", Port: 22, Username: "root"},
	}}}, false)
	if !errors.Is(err, errInvalidTeamImport) || !strings.Contains(err.Error(), "requires explicit confirmation") {
		t.Fatalf("expected invalid import host key error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTeamImportRejectsInvalidAgentPublicKeyBeforeTransaction(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() { db.Db = oldDB })
	privateKey, _, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = applyTeamImport(5, teamExportBundle{Servers: []teamExportServer{{
		Type: 1,
		Name: "broken-active",
		Agent: &teamExportAgent{
			PrivateKey: privateKey,
			PublicKey:  "not a PEM key",
		},
	}}}, false)
	if !errors.Is(err, errInvalidTeamImport) || !strings.Contains(err.Error(), "broken-active") {
		t.Fatalf("expected invalid Agent public key error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTeamImportRejectsInvalidAgentPrivateKeyBeforeTransaction(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() { db.Db = oldDB })

	_, _, err = applyTeamImport(5, teamExportBundle{Servers: []teamExportServer{{
		Type: 1,
		Name: "broken-active",
		Agent: &teamExportAgent{
			PrivateKey: "not a PEM key",
		},
	}}}, false)
	if !errors.Is(err, errInvalidTeamImport) || !strings.Contains(err.Error(), "invalid Agent private key") {
		t.Fatalf("expected invalid Agent private key error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightImportedAgentUIDsRejectsDuplicateBundle(t *testing.T) {
	err := preflightImportedAgentUIDs(5, []teamExportServer{
		{Name: "first", Type: 2, Agent: &teamExportAgent{AgentUID: "duplicate-uid"}},
		{Name: "second", Type: 2, Agent: &teamExportAgent{AgentUID: "duplicate-uid"}},
	})
	var conflict *teamImportAgentUIDConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T %v, want teamImportAgentUIDConflict", err, err)
	}
	if conflict.ServerName != "second" || conflict.ConflictingServerName != "first" || conflict.AgentUID != "duplicate-uid" || conflict.ConflictSource != "bundle" {
		t.Fatalf("conflict = %#v", conflict)
	}
}

func TestPreflightImportedAgentUIDsFindsCrossTeamConflict(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() { db.Db = oldDB })
	mock.ExpectQuery(`SELECT btrim\(a.agent_uid\).*s.team_id <> \$2`).
		WithArgs(sqlmock.AnyArg(), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"agent_uid"}).AddRow("existing-uid"))

	err = preflightImportedAgentUIDs(5, []teamExportServer{{
		Name: "imported-active", Type: 2, Agent: &teamExportAgent{AgentUID: "existing-uid"},
	}})
	var conflict *teamImportAgentUIDConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T %v, want teamImportAgentUIDConflict", err, err)
	}
	if conflict.ServerName != "imported-active" || conflict.AgentUID != "existing-uid" || conflict.ConflictSource != "database" {
		t.Fatalf("conflict = %#v", conflict)
	}
}

func TestPreflightImportedAgentUIDsAllowsCurrentTeamIdentity(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() { db.Db = oldDB })
	mock.ExpectQuery(`SELECT btrim\(a.agent_uid\).*s.team_id <> \$2`).
		WithArgs(sqlmock.AnyArg(), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"agent_uid"}))

	if err = preflightImportedAgentUIDs(5, []teamExportServer{{
		Name: "same-team", Type: 2, Agent: &teamExportAgent{AgentUID: "same-team-uid"},
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestAgentUIDConflictResponseIdentifiesImportedServer(t *testing.T) {
	e := echo.New()
	e.GET("/conflict", func(c *echo.Context) error {
		return agentUIDConflictResponse(c, &teamImportAgentUIDConflict{
			ServerName: "imported-active", AgentUID: "existing-uid", ConflictSource: "database",
		})
	})
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/conflict", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	for _, fragment := range []string{
		`"code":"agent_uid_conflict"`,
		`"server_name":"imported-active"`,
		`"agent_uid":"existing-uid"`,
		`"conflict_source":"database"`,
	} {
		if !strings.Contains(recorder.Body.String(), fragment) {
			t.Fatalf("response does not contain %q: %s", fragment, recorder.Body.String())
		}
	}
}

func TestImportServerConnectionMapsConcurrentAgentUIDConflict(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	mock.ExpectBegin()
	tx, err := sqlx.NewDb(database, "sqlmock").Beginx()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`INSERT INTO agents .*private_key`).
		WillReturnError(&pq.Error{Code: "23505", Constraint: "IDX_AAU"})
	mock.ExpectRollback()

	err = importServerConnection(tx, 91, teamExportServer{
		Name: "racing-passive", Type: 2, Agent: &teamExportAgent{AgentUID: "racing-uid"},
	}, nil, false)
	var conflict *teamImportAgentUIDConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T %v, want teamImportAgentUIDConflict", err, err)
	}
	if conflict.ServerName != "racing-passive" || conflict.AgentUID != "racing-uid" || conflict.ConflictSource != "database" {
		t.Fatalf("conflict = %#v", conflict)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeTeamImportBundleEncrypted(t *testing.T) {
	bundle := testTeamExportBundle()
	encrypted, err := exportcrypto.EncryptJSON("export-pass-123", bundle)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeTeamImportBundle(teamImportRequest{
		ExportPassword: "export-pass-123",
		Encrypted:      encrypted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != teamExportVersion || got.Team.Name != bundle.Team.Name {
		t.Fatalf("got %#v", got)
	}
}

func TestImportSSHConnectionPreservesPinnedHostKey(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	mock.ExpectBegin()
	tx, err := sqlx.NewDb(database, "sqlmock").Beginx()
	if err != nil {
		t.Fatal(err)
	}

	passwordMatcher := sqlmock.AnyArg()
	hostKey := newExportTestHostKey(t)
	mock.ExpectExec(`INSERT INTO ssh .*host_key, trust_legacy_host_key`).
		WithArgs(int64(91), "server.example", 22, "root", int64(17), passwordMatcher, hostKey, false).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	oldKey := encrypt.Key
	encrypt.Key = []byte("0123456789abcdef0123456789abcdef")
	t.Cleanup(func() { encrypt.Key = oldKey })
	err = importServerConnection(tx, 91, teamExportServer{
		Type: 0,
		Name: "server",
		SSH: &teamExportSSH{
			Address:  "server.example",
			Port:     22,
			Username: "root",
			KeyRef:   7,
			Password: "secret",
			HostKey:  hostKey + " imported-comment",
		},
	}, map[int64]int64{7: 17}, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyImportWithoutSSHHostKeyRequiresConfirmation(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	mock.ExpectBegin()
	tx, err := sqlx.NewDb(database, "sqlmock").Beginx()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectRollback()

	oldKey := encrypt.Key
	encrypt.Key = []byte("0123456789abcdef0123456789abcdef")
	t.Cleanup(func() { encrypt.Key = oldKey })
	err = importServerConnection(tx, 92, teamExportServer{
		Type: 0,
		Name: "legacy",
		SSH: &teamExportSSH{
			Address:  "legacy.example",
			Port:     22,
			Username: "root",
			Password: "secret",
		},
	}, map[int64]int64{}, false)
	if err == nil || !strings.Contains(err.Error(), "requires explicit confirmation") {
		t.Fatalf("expected missing host key error, got %v", err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmedLegacyImportTrustsSSHHostKey(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	mock.ExpectBegin()
	tx, err := sqlx.NewDb(database, "sqlmock").Beginx()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`INSERT INTO ssh .*host_key, trust_legacy_host_key`).
		WithArgs(int64(92), "legacy.example", 22, "root", int64(0), sqlmock.AnyArg(), nil, true).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	oldKey := encrypt.Key
	encrypt.Key = []byte("0123456789abcdef0123456789abcdef")
	t.Cleanup(func() { encrypt.Key = oldKey })
	err = importServerConnection(tx, 92, teamExportServer{
		Type: 0,
		Name: "legacy",
		SSH: &teamExportSSH{
			Address:  "legacy.example",
			Port:     22,
			Username: "root",
			Password: "secret",
		},
	}, map[int64]int64{}, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestImportActiveAgentEncryptsPrivateKeyForNewServer(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	mock.ExpectBegin()
	tx, err := sqlx.NewDb(database, "sqlmock").Beginx()
	if err != nil {
		t.Fatal(err)
	}
	privateKey, _, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	oldKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() { encrypt.Key = oldKey })

	mock.ExpectExec(`INSERT INTO agents .*private_key`).
		WithArgs(
			int64(91), "agent-uid", int16(0), sqlmock.AnyArg(), "", "", "",
			agentPrivateKeyCiphertextMatcher{serverID: 91, plaintext: []byte(privateKey)},
			int16(1), "agent.example.com", 443,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	err = importServerConnection(tx, 91, teamExportServer{
		Type: 1,
		Name: "active",
		Agent: &teamExportAgent{
			AgentUID:   "agent-uid",
			PrivateKey: privateKey,
			Host:       "agent.example.com",
			Port:       443,
		},
	}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExportActiveAgentDecryptsPrivateKey(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	oldKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() {
		db.Db = oldDB
		encrypt.Key = oldKey
	})
	privateKey, _, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := encrypt.Encrypt([]byte(privateKey), encrypt.Key, encrypt.AgentPrivateKeyContext(91))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT agent_uid, status, last_seen_at, last_ip, last_version, public_key, private_key, protocol_version, host, port`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"agent_uid", "status", "last_seen_at", "last_ip", "last_version", "public_key", "private_key", "protocol_version", "host", "port"}).
			AddRow("agent-uid", int16(1), nil, "", "", "", ciphertext, int16(2), "agent.example.com", 443))
	mock.ExpectQuery(`SELECT token_hash, is_revoked, created_at FROM enroll_tokens`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"token_hash", "is_revoked", "created_at"}))

	server := &teamExportServer{Type: 1, RefID: 91}
	if err = exportServerConnection(server); err != nil {
		t.Fatal(err)
	}
	if server.Agent == nil || server.Agent.PrivateKey != privateKey {
		t.Fatal("Active Agent export did not restore the private PEM")
	}
}

func TestExportActiveAgentRejectsSwappedPrivateKey(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	oldKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() {
		db.Db = oldDB
		encrypt.Key = oldKey
	})
	ciphertext, err := encrypt.Encrypt([]byte("private-key"), encrypt.Key, encrypt.AgentPrivateKeyContext(92))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT agent_uid, status, last_seen_at, last_ip, last_version, public_key, private_key, protocol_version, host, port`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"agent_uid", "status", "last_seen_at", "last_ip", "last_version", "public_key", "private_key", "protocol_version", "host", "port"}).
			AddRow("agent-uid", int16(1), nil, "", "", "", ciphertext, int16(2), "agent.example.com", 443))

	server := &teamExportServer{Type: 1, RefID: 91, Name: "broken-active"}
	err = exportServerConnection(server)
	if err == nil {
		t.Fatal("export accepted an Agent private key bound to another Server")
	}
	var credentialErr *teamExportCredentialError
	if !errors.As(err, &credentialErr) {
		t.Fatalf("error = %T %v, want teamExportCredentialError", err, err)
	}
	if credentialErr.ServerID != 91 || credentialErr.ServerName != "broken-active" || credentialErr.Credential != "active_agent_private_key" {
		t.Fatalf("credential error = %#v", credentialErr)
	}
}

func TestUnreadableServerCredentialConflictIdentifiesServer(t *testing.T) {
	e := echo.New()
	e.GET("/conflict", func(c *echo.Context) error {
		return unreadableServerCredentialConflict(c, &teamExportCredentialError{
			ServerID:   91,
			ServerName: "broken-active",
			Credential: "active_agent_private_key",
			Err:        errors.New("authentication failed"),
		})
	})
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/conflict", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	for _, fragment := range []string{
		`"code":"unreadable_server_credential"`,
		`"server_id":91`,
		`"server_name":"broken-active"`,
		`"credential":"active_agent_private_key"`,
	} {
		if !strings.Contains(recorder.Body.String(), fragment) {
			t.Fatalf("response does not contain %q: %s", fragment, recorder.Body.String())
		}
	}
}

func TestUnreadableKeyCredentialConflictIdentifiesKey(t *testing.T) {
	e := echo.New()
	e.GET("/conflict", func(c *echo.Context) error {
		return unreadableKeyCredentialConflict(c, &teamExportKeyCredentialError{
			KeyID:      44,
			KeyName:    "production-key",
			Credential: "ssh_key_content",
			Err:        errors.New("authentication failed"),
		})
	})
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/conflict", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	for _, fragment := range []string{
		`"code":"unreadable_key_credential"`,
		`"key_id":44`,
		`"key_name":"production-key"`,
		`"credential":"ssh_key_content"`,
	} {
		if !strings.Contains(recorder.Body.String(), fragment) {
			t.Fatalf("response does not contain %q: %s", fragment, recorder.Body.String())
		}
	}
}

func TestExportKeysCanSkipUnreadableCredential(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	oldKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() {
		db.Db = oldDB
		encrypt.Key = oldKey
	})

	healthyContent, err := encrypt.Encrypt([]byte("healthy-private-key"), encrypt.Key, encrypt.KeyContentContext(45))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	mock.ExpectQuery(`SELECT id AS ref_id, name, content, password, created_at, updated_at FROM keys`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"ref_id", "name", "content", "password", "created_at", "updated_at"}).
			AddRow(int64(44), "broken-key", []byte("plaintext-key"), nil, now, now).
			AddRow(int64(45), "healthy-key", healthyContent, nil, now, now))

	var data teamExportBundle
	unreadable, err := exportKeys(7, &data, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := unreadable[44]; !ok || len(unreadable) != 1 {
		t.Fatalf("unreadable Key IDs = %#v", unreadable)
	}
	if len(data.SkippedKeys) != 1 || data.SkippedKeys[0].KeyID != 44 || data.SkippedKeys[0].KeyName != "broken-key" || data.SkippedKeys[0].Credential != "ssh_key_content" {
		t.Fatalf("skipped Keys = %#v", data.SkippedKeys)
	}
	if len(data.Keys) != 1 || data.Keys[0].RefID != 45 || data.Keys[0].Content != "healthy-private-key" {
		t.Fatalf("exported Keys = %#v", data.Keys)
	}
}

func TestExportKeysStrictModeIdentifiesUnreadableCredential(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	oldKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() {
		db.Db = oldDB
		encrypt.Key = oldKey
	})

	now := time.Now()
	mock.ExpectQuery(`SELECT id AS ref_id, name, content, password, created_at, updated_at FROM keys`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"ref_id", "name", "content", "password", "created_at", "updated_at"}).
			AddRow(int64(44), "broken-key", []byte("plaintext-key"), nil, now, now))

	var data teamExportBundle
	_, err = exportKeys(7, &data, false)
	var credentialErr *teamExportKeyCredentialError
	if !errors.As(err, &credentialErr) {
		t.Fatalf("error = %T %v, want teamExportKeyCredentialError", err, err)
	}
	if credentialErr.KeyID != 44 || credentialErr.KeyName != "broken-key" || credentialErr.Credential != "ssh_key_content" {
		t.Fatalf("credential error = %#v", credentialErr)
	}
}

func TestTeamExportUnreadableServerPolicyDefaultsToSkip(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "omitted", raw: `{}`, want: true},
		{name: "explicit skip", raw: `{"skip_unreadable_servers":true}`, want: true},
		{name: "strict export", raw: `{"skip_unreadable_servers":false}`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request teamExportAuthRequest
			if err := json.Unmarshal([]byte(tt.raw), &request); err != nil {
				t.Fatal(err)
			}
			if got := request.shouldSkipUnreadableServers(); got != tt.want {
				t.Fatalf("shouldSkipUnreadableServers() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestExportServersCanSkipUnreadableCredential(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	oldKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() {
		db.Db = oldDB
		encrypt.Key = oldKey
	})

	mock.ExpectQuery(`SELECT id AS ref_id, category AS category_ref, type, name, allow_monitor, allow_terminal, public_visible, weight`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"ref_id", "category_ref", "type", "name", "allow_monitor", "allow_terminal", "weight"}).
			AddRow(int64(91), int64(1), int16(1), "broken-active", true, true, 0).
			AddRow(int64(92), int64(1), int16(2), "healthy-passive", true, true, 0))
	expectEmptyExportServerInfo(mock, 91)
	mock.ExpectQuery(`SELECT agent_uid, status, last_seen_at, last_ip, last_version, public_key, private_key, protocol_version, host, port`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"agent_uid", "status", "last_seen_at", "last_ip", "last_version", "public_key", "private_key", "protocol_version", "host", "port"}).
			AddRow("agent-uid", int16(1), nil, "", "", "", []byte("plaintext-key"), int16(2), "agent.example.com", 443))
	expectEmptyExportServerInfo(mock, 92)
	mock.ExpectQuery(`SELECT agent_uid, status, last_seen_at, last_ip, last_version, public_key, private_key, protocol_version, host, port`).
		WithArgs(int64(92)).
		WillReturnRows(sqlmock.NewRows([]string{"agent_uid", "status", "last_seen_at", "last_ip", "last_version", "public_key", "private_key", "protocol_version", "host", "port"}))
	mock.ExpectQuery(`SELECT token_hash, is_revoked, created_at FROM enroll_tokens`).
		WithArgs(int64(92)).
		WillReturnRows(sqlmock.NewRows([]string{"token_hash", "is_revoked", "created_at"}))
	mock.ExpectQuery(`SELECT item, threshold, for_duration`).
		WithArgs(int64(92)).
		WillReturnRows(sqlmock.NewRows([]string{"item", "threshold", "for_duration"}))

	var data teamExportBundle
	skipped, err := exportServers(7, &data, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 || skipped[0].ServerID != 91 || skipped[0].ServerName != "broken-active" || skipped[0].Credential != "active_agent_private_key" {
		t.Fatalf("skipped Servers = %#v", skipped)
	}
	if len(data.Servers) != 1 || data.Servers[0].RefID != 92 {
		t.Fatalf("exported Servers = %#v, want only Server 92", data.Servers)
	}
}

func TestExportServersSkipsServerDependingOnUnreadableKey(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	oldKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() {
		db.Db = oldDB
		encrypt.Key = oldKey
	})

	password, err := encrypt.Encrypt([]byte("ssh-password"), encrypt.Key, encrypt.SSHPasswordContext(91))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT id AS ref_id, category AS category_ref, type, name, allow_monitor, allow_terminal, public_visible, weight`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"ref_id", "category_ref", "type", "name", "allow_monitor", "allow_terminal", "weight"}).
			AddRow(int64(91), int64(1), int16(0), "dependent-ssh", true, true, 0))
	expectEmptyExportServerInfo(mock, 91)
	mock.ExpectQuery(`SELECT address, port, username, key_id AS key_ref, password`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"address", "port", "username", "key_ref", "password", "host_key"}).
			AddRow("ssh.example.com", 22, "root", int64(44), password, ""))

	var data teamExportBundle
	skipped, err := exportServers(7, &data, true, map[int64]struct{}{44: {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 || skipped[0].ServerID != 91 || skipped[0].ServerName != "dependent-ssh" || skipped[0].Credential != "ssh_key" {
		t.Fatalf("skipped Servers = %#v", skipped)
	}
	if len(data.Servers) != 0 {
		t.Fatalf("exported Servers = %#v, want none", data.Servers)
	}
}

func expectEmptyExportServerInfo(mock sqlmock.Sqlmock, serverID int64) {
	mock.ExpectQuery(`SELECT os, county, area, open_time, note, provider, cycle, start_time, end_time, amount,`).
		WithArgs(int64(serverID)).
		WillReturnRows(sqlmock.NewRows([]string{"os", "county", "area", "open_time", "note", "provider", "cycle", "start_time", "end_time", "amount", "auto_renew", "bandwidth", "traffic", "traffic_type", "note_public", "online"}))
	mock.ExpectQuery(`SELECT hostname, cpu_name, core_c, core_t, kernel, ip, arch`).
		WithArgs(int64(serverID)).
		WillReturnRows(sqlmock.NewRows([]string{"hostname", "cpu_name", "core_c", "core_t", "kernel", "ip", "arch"}))
}

func TestInspectImportedSSHHostKeysReturnsLegacyServers(t *testing.T) {
	hostKey := newExportTestHostKey(t)
	servers, err := inspectImportedSSHHostKeys([]teamExportServer{
		{Type: 0, Name: "legacy-a", SSH: &teamExportSSH{}},
		{Type: 0, Name: "pinned", SSH: &teamExportSSH{HostKey: hostKey}},
		{Type: 1, Name: "agent"},
		{Type: 0, Name: "legacy-b", SSH: &teamExportSSH{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || servers[0] != "legacy-a" || servers[1] != "legacy-b" {
		t.Fatalf("legacy servers = %#v", servers)
	}
}

func TestImportSSHConnectionRejectsMalformedHostKey(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	mock.ExpectBegin()
	tx, err := sqlx.NewDb(database, "sqlmock").Beginx()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectRollback()

	oldKey := encrypt.Key
	encrypt.Key = []byte("0123456789abcdef0123456789abcdef")
	t.Cleanup(func() { encrypt.Key = oldKey })
	err = importServerConnection(tx, 93, teamExportServer{
		Type: 0,
		Name: "malformed",
		SSH: &teamExportSSH{
			Address:  "malformed.example",
			Port:     22,
			Username: "root",
			Password: "secret",
			HostKey:  "not-an-ssh-public-key",
		},
	}, map[int64]int64{}, false)
	if err == nil || !strings.Contains(err.Error(), "invalid SSH host key") {
		t.Fatalf("error = %v", err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeImportedPassiveServerRuntime(t *testing.T) {
	osName := "Linux"
	county := "HK"
	area := "Hong Kong"
	hostname := "server-1"
	ip := "203.0.113.10"
	note := "keep this note"
	openTime := time.Now().Add(-time.Hour)

	info := normalizeImportedServerInfo(2, teamExportServerInfo{
		OS:       &osName,
		County:   &county,
		Area:     &area,
		OpenTime: &openTime,
		Note:     &note,
		Online:   true,
	})
	if info.OS != nil || info.County != nil || info.Area != nil || info.OpenTime != nil || info.Online {
		t.Fatalf("passive runtime info was not cleared: %#v", info)
	}
	if info.Note == nil || *info.Note != note {
		t.Fatalf("static server info was not preserved: %#v", info.Note)
	}

	advanced := normalizeImportedServerAdvancedInfo(2, teamExportServerInfoAdv{
		Hostname: &hostname,
		IP:       &ip,
	})
	if advanced.Hostname != nil || advanced.IP != nil {
		t.Fatalf("passive advanced info was not cleared: %#v", advanced)
	}

	lastSeen := time.Now().Add(-time.Minute)
	seen, lastIP, version := normalizeImportedAgentRuntime(2, &teamExportAgent{
		LastSeenAt:  &lastSeen,
		LastIP:      ip,
		LastVersion: "v1.2.3",
	})
	if seen != nil || lastIP != "" || version != "" {
		t.Fatalf("passive agent runtime was not cleared: seen=%v ip=%q version=%q", seen, lastIP, version)
	}
}

func TestNormalizeImportedActiveServerPreservesRuntime(t *testing.T) {
	osName := "Linux"
	ip := "203.0.113.10"
	lastSeen := time.Now().Add(-time.Minute)

	info := normalizeImportedServerInfo(1, teamExportServerInfo{OS: &osName, Online: true})
	advanced := normalizeImportedServerAdvancedInfo(1, teamExportServerInfoAdv{IP: &ip})
	seen, lastIP, version := normalizeImportedAgentRuntime(1, &teamExportAgent{
		LastSeenAt:  &lastSeen,
		LastIP:      ip,
		LastVersion: "v1.2.3",
	})
	if info.OS == nil || *info.OS != osName || !info.Online || advanced.IP == nil || *advanced.IP != ip {
		t.Fatal("active server runtime should be preserved")
	}
	if seen == nil || !seen.Equal(lastSeen) || lastIP != ip || version != "v1.2.3" {
		t.Fatal("active agent runtime should be preserved")
	}
}

func TestNormalizeImportedAgentProtocol(t *testing.T) {
	tests := []struct {
		name       string
		serverType int16
		agent      teamExportAgent
		want       int16
	}{
		{name: "explicit legacy is preserved", serverType: 1, agent: teamExportAgent{ProtocolVersion: 1}, want: 1},
		{name: "explicit v2", serverType: 1, agent: teamExportAgent{ProtocolVersion: 2}, want: 2},
		{name: "pinned key overrides explicit legacy", serverType: 1, agent: teamExportAgent{ProtocolVersion: 1, PublicKey: "pinned"}, want: 2},
		{name: "old unpaired active export", serverType: 1, agent: teamExportAgent{}, want: 1},
		{name: "old pinned active export", serverType: 1, agent: teamExportAgent{PublicKey: "pinned"}, want: 2},
		{name: "passive export", serverType: 2, agent: teamExportAgent{}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeImportedAgentProtocol(tt.serverType, &tt.agent); got != tt.want {
				t.Fatalf("normalizeImportedAgentProtocol() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNormalizeImportedAgentKeys(t *testing.T) {
	privateKey, publicKey, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	servers := []teamExportServer{
		{Name: "pinned", Type: 1, Agent: &teamExportAgent{PrivateKey: " \n" + privateKey + "\t", PublicKey: " \n" + publicKey + "\t", ProtocolVersion: 1}},
		{Name: "unpaired", Type: 1, Agent: &teamExportAgent{PrivateKey: privateKey, PublicKey: " \n\t"}},
		{Name: "passive", Type: 2, Agent: &teamExportAgent{PrivateKey: privateKey}},
	}
	normalized, err := normalizeImportedAgentKeys(servers)
	if err != nil {
		t.Fatal(err)
	}
	if normalized[0].Agent.PublicKey != publicKey {
		t.Fatal("valid imported public key was not canonicalized")
	}
	if got := normalizeImportedAgentProtocol(normalized[0].Type, normalized[0].Agent); got != 2 {
		t.Fatalf("pinned protocol version = %d, want 2", got)
	}
	if normalized[1].Agent.PublicKey != "" {
		t.Fatalf("blank imported public key = %q, want empty", normalized[1].Agent.PublicKey)
	}
	if normalized[0].Agent.PrivateKey != privateKey {
		t.Fatal("valid imported private key was not normalized")
	}
	if normalized[2].Agent.PrivateKey != "" {
		t.Fatal("Passive Agent import retained a private key")
	}
	if servers[1].Agent.PublicKey == "" {
		t.Fatal("normalization mutated the input bundle")
	}
}

type agentPrivateKeyCiphertextMatcher struct {
	serverID  int64
	plaintext []byte
}

func (matcher agentPrivateKeyCiphertextMatcher) Match(value driver.Value) bool {
	ciphertext, ok := value.([]byte)
	if !ok || bytes.Contains(ciphertext, []byte("BEGIN PRIVATE KEY")) {
		return false
	}
	plaintext, err := encrypt.Decrypt(ciphertext, encrypt.Key, encrypt.AgentPrivateKeyContext(matcher.serverID))
	return err == nil && bytes.Equal(plaintext, matcher.plaintext)
}

func TestDecodeTeamImportBundlePlaintextIgnoresPassword(t *testing.T) {
	bundle := testTeamExportBundle()
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}

	got, err := decodeTeamImportBundle(teamImportRequest{
		ExportPassword: "short",
		Data:           raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != teamExportVersion || got.Team.Name != bundle.Team.Name {
		t.Fatalf("got %#v", got)
	}
}

func TestDecodeTeamImportBundleDataTakesPriorityOverEncrypted(t *testing.T) {
	plaintext := testTeamExportBundle()
	plaintext.Team.Name = "From Plaintext Data"
	raw, err := json.Marshal(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	other := testTeamExportBundle()
	other.Team.Name = "From Encrypted"
	encrypted, err := exportcrypto.EncryptJSON("export-pass-123", other)
	if err != nil {
		t.Fatal(err)
	}

	got, err := decodeTeamImportBundle(teamImportRequest{
		ExportPassword: "wrong-password",
		Encrypted:      encrypted,
		Data:           raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Team.Name != "From Plaintext Data" {
		t.Fatalf("expected plaintext branch, got %#v", got)
	}
}

func TestDecodeTeamImportBundleMissingBoth(t *testing.T) {
	_, err := decodeTeamImportBundle(teamImportRequest{
		ExportPassword: "export-pass-123",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing encrypted export payload") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeTeamImportBundleNullDataUsesEncrypted(t *testing.T) {
	bundle := testTeamExportBundle()
	bundle.Team.Name = "From Encrypted Only"
	encrypted, err := exportcrypto.EncryptJSON("export-pass-123", bundle)
	if err != nil {
		t.Fatal(err)
	}

	got, err := decodeTeamImportBundle(teamImportRequest{
		ExportPassword: "export-pass-123",
		Encrypted:      encrypted,
		Data:           json.RawMessage("null"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Team.Name != "From Encrypted Only" {
		t.Fatalf("expected encrypted branch, got %#v", got)
	}
}

func TestDecodeTeamImportBundleWhitespaceDataUsesEncrypted(t *testing.T) {
	bundle := testTeamExportBundle()
	encrypted, err := exportcrypto.EncryptJSON("export-pass-123", bundle)
	if err != nil {
		t.Fatal(err)
	}

	for _, raw := range []json.RawMessage{nil, json.RawMessage(""), json.RawMessage("   \n\t")} {
		got, err := decodeTeamImportBundle(teamImportRequest{
			ExportPassword: "export-pass-123",
			Encrypted:      encrypted,
			Data:           raw,
		})
		if err != nil {
			t.Fatalf("data=%q: %v", raw, err)
		}
		if got.Team.Name != bundle.Team.Name {
			t.Fatalf("data=%q: got %#v", raw, got)
		}
	}
}

func testTeamExportBundle() teamExportBundle {
	return teamExportBundle{
		Version: teamExportVersion,
		Team: teamExportTeam{
			Name:        "Legacy Team",
			Description: "imported from plaintext export",
			Color:       "#2563eb",
			Image:       "",
		},
		Categories: []teamExportCategory{
			{RefID: 1, Name: "Default", Sort: 0},
		},
		Servers: []teamExportServer{},
	}
}

func newExportTestHostKey(t *testing.T) string {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(gossh.MarshalAuthorizedKey(signer.PublicKey())))
}
