package ateam

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mosona-manager/internal/security/exportcrypto"
	"mosona-manager/internal/utils/encrypt"
	"mosona-manager/pkg/identity"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

// legacyTeamExportRawJSON builds a bundle exactly as a v0.1.14-era Hub would
// have written it: no protocol_version, no public_visible, no
// skipped_servers/skipped_keys, plaintext Agent private keys in the bundle,
// and SSH servers without pinned host keys. It also returns the Active Agent
// private key embedded in the bundle so callers can assert on it.
func legacyTeamExportRawJSON(t *testing.T) (string, string) {
	t.Helper()
	activePrivateKey, passivePublicKey, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	bundle := map[string]any{
		"version":       1,
		"exported_at":   time.Now().UTC().Format(time.RFC3339),
		"team":          map[string]any{"name": "Legacy Team", "description": "", "color": "#2563eb", "image": ""},
		"categories":    []any{map[string]any{"ref_id": 1, "name": "Default", "sort": 0}},
		"keys":          []any{},
		"team_alerts":   []any{},
		"notifications": []any{},
		"servers": []any{
			map[string]any{
				"ref_id": 11, "category_ref": 1, "type": 0, "name": "legacy-ssh",
				"allow_monitor": true, "allow_terminal": true, "weight": 0,
				"ssh": map[string]any{
					"address": "legacy.example", "port": 22, "username": "root",
					"key_ref": 0, "password": "secret",
				},
			},
			map[string]any{
				"ref_id": 12, "category_ref": 1, "type": 1, "name": "legacy-active",
				"allow_monitor": true, "allow_terminal": true, "weight": 0,
				"agent": map[string]any{
					"agent_uid": "legacy-active-uid", "status": 1,
					"public_key": "", "private_key": activePrivateKey,
				},
			},
			map[string]any{
				"ref_id": 13, "category_ref": 1, "type": 2, "name": "legacy-passive",
				"allow_monitor": true, "allow_terminal": true, "weight": 0,
				"agent": map[string]any{
					"agent_uid": "legacy-passive-uid", "status": 1,
					"public_key": passivePublicKey, "private_key": "",
				},
			},
		},
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), activePrivateKey
}

func decodeLegacyBundle(t *testing.T) (teamExportBundle, string) {
	t.Helper()
	raw, activePrivateKey := legacyTeamExportRawJSON(t)
	encrypted, err := exportcrypto.EncryptJSON("export-pass-123", json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := decodeTeamImportBundle(teamImportRequest{
		ExportPassword: "export-pass-123",
		Encrypted:      encrypted,
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle, activePrivateKey
}

// TestLegacyBundleDecodesIntoCurrentStructs proves bundles written before
// protocol_version/public_visible existed still decode, with the expected
// zero values that drive the compatibility defaults.
func TestLegacyBundleDecodesIntoCurrentStructs(t *testing.T) {
	bundle, activePrivateKey := decodeLegacyBundle(t)
	if bundle.Version != teamExportVersion {
		t.Fatalf("version = %d", bundle.Version)
	}
	if len(bundle.Servers) != 3 {
		t.Fatalf("servers = %d", len(bundle.Servers))
	}
	ssh, active, passive := bundle.Servers[0], bundle.Servers[1], bundle.Servers[2]
	if ssh.SSH == nil || ssh.SSH.HostKey != "" {
		t.Fatalf("legacy SSH host key = %#v", ssh.SSH)
	}
	if active.Agent.ProtocolVersion != 0 || active.PublicVisible != nil {
		t.Fatalf("legacy active agent protocol/public_visible = %d/%#v", active.Agent.ProtocolVersion, active.PublicVisible)
	}
	if active.Agent.PrivateKey != activePrivateKey {
		t.Fatal("legacy Active Agent private key was altered during decode")
	}
	if passive.Agent.PrivateKey != "" || passive.PublicVisible != nil {
		t.Fatalf("legacy passive agent private key not empty or public_visible set")
	}
	if bundle.SkippedServers != nil || bundle.SkippedKeys != nil {
		t.Fatalf("legacy bundle must not report skipped entries")
	}
}

func expectImportedServerRows(mock sqlmock.Sqlmock, serverID int64) {
	mock.ExpectQuery(`INSERT INTO servers .*public_visible, weight\)`).
		WithArgs(int64(7), "legacy-ssh", int16(0), int64(5), true, true, true, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(serverID))
	expectImportServerInfo(mock, serverID)
	expectImportServerAdvancedInfo(mock, serverID)
	mock.ExpectExec(`INSERT INTO ssh .*trust_legacy_host_key`).
		WithArgs(serverID, "legacy.example", 22, "root", int64(0), sqlmock.AnyArg(), nil, true).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectImportServerInfo(mock sqlmock.Sqlmock, serverID int64) {
	args := make([]driver.Value, 17)
	args[0] = serverID
	for i := 1; i < len(args); i++ {
		args[i] = sqlmock.AnyArg()
	}
	mock.ExpectExec(`INSERT INTO server_info `).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectImportServerAdvancedInfo(mock sqlmock.Sqlmock, serverID int64) {
	args := make([]driver.Value, 8)
	args[0] = serverID
	for i := 1; i < len(args); i++ {
		args[i] = sqlmock.AnyArg()
	}
	mock.ExpectExec(`INSERT INTO server_info_adv `).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// TestLegacyBundleImportsWithCompatibilityDefaults runs importServers on a
// decoded legacy bundle plus one new-style hidden server and asserts:
//   - legacy servers (public_visible absent) import as visible;
//   - new-style hidden servers import as hidden;
//   - unpinned legacy Active Agents import as protocol v1 (upgrade window)
//     while pinned identities import as v2;
//   - Agent private keys are re-encrypted under the new server identity.
func TestLegacyBundleImportsWithCompatibilityDefaults(t *testing.T) {
	privateKey, publicKey, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	hidden := false
	newStyle := teamExportServer{
		RefID: 14, CategoryRef: 1, Type: 1, Name: "new-style-hidden",
		AllowMonitor: true, AllowTerminal: true, PublicVisible: &hidden,
		Agent: &teamExportAgent{
			AgentUID: "new-style-uid", Status: 1,
			PublicKey: publicKey, PrivateKey: privateKey,
			ProtocolVersion: 2, Host: "agent.example.com", Port: 443,
		},
	}

	bundle, legacyActivePrivateKey := decodeLegacyBundle(t)
	servers := append(bundle.Servers, newStyle)
	servers, err = normalizeImportedAgentKeys(servers)
	if err != nil {
		t.Fatal(err)
	}
	legacyPEM := strings.TrimSpace(legacyActivePrivateKey) + "\n"
	if servers[1].Agent.PrivateKey != legacyPEM {
		t.Fatalf("legacy private key not normalized: %q", servers[1].Agent.PrivateKey)
	}
	if servers[2].Agent.PrivateKey != "" {
		t.Fatalf("passive private key not cleared on import")
	}

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
	oldKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() { encrypt.Key = oldKey })

	expectImportedServerRows(mock, 101)
	mock.ExpectQuery(`INSERT INTO servers .*public_visible, weight\)`).
		WithArgs(int64(7), "legacy-active", int16(1), int64(5), true, true, true, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(102)))
	expectImportServerInfo(mock, 102)
	expectImportServerAdvancedInfo(mock, 102)
	mock.ExpectExec(`INSERT INTO agents `).
		WithArgs(
			int64(102), "legacy-active-uid", int16(1), sqlmock.AnyArg(), "", "", "",
			agentPrivateKeyCiphertextMatcher{serverID: 102, plaintext: []byte(legacyPEM)},
			int16(1), "", 0,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO servers .*public_visible, weight\)`).
		WithArgs(int64(7), "legacy-passive", int16(2), int64(5), true, true, true, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(103)))
	expectImportServerInfo(mock, 103)
	expectImportServerAdvancedInfo(mock, 103)
	mock.ExpectExec(`INSERT INTO agents `).
		WithArgs(
			int64(103), "legacy-passive-uid", int16(1), sqlmock.AnyArg(), "", "", sqlmock.AnyArg(),
			[]byte{}, int16(2), "", 0,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO servers .*public_visible, weight\)`).
		WithArgs(int64(7), "new-style-hidden", int16(1), int64(5), true, true, false, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(104)))
	expectImportServerInfo(mock, 104)
	expectImportServerAdvancedInfo(mock, 104)
	mock.ExpectExec(`INSERT INTO agents `).
		WithArgs(
			int64(104), "new-style-uid", int16(1), sqlmock.AnyArg(), "", "", sqlmock.AnyArg(),
			agentPrivateKeyCiphertextMatcher{serverID: 104, plaintext: []byte(strings.TrimSpace(privateKey) + "\n")},
			int16(2), "agent.example.com", 443,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	if _, err = importServers(tx, 7, servers, map[int64]int64{1: 5}, map[int64]int64{}, true); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
