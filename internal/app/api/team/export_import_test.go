package ateam

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/notification"
	"mosona-manager/internal/security/exportcrypto"
	"mosona-manager/internal/utils/encrypt"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	gossh "golang.org/x/crypto/ssh"
)

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
		Module: "shoutrrr", Target: "generic://169.254.169.254/latest/meta-data",
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
			Module: "shoutrrr", Target: "generic://127.0.0.1/hook",
		}},
	})
	if !errors.Is(err, notification.ErrInvalidConfiguration) {
		t.Fatalf("error = %v", err)
	}
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
	mock.ExpectExec(`INSERT INTO ssh .*host_key.*NULLIF`).
		WithArgs(int64(91), "server.example", 22, "root", int64(17), passwordMatcher, hostKey).
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
	}, map[int64]int64{7: 17})
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyImportLeavesSSHHostKeyUntrusted(t *testing.T) {
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
	mock.ExpectExec(`INSERT INTO ssh .*host_key.*NULLIF`).
		WithArgs(int64(92), "legacy.example", 22, "root", int64(0), sqlmock.AnyArg(), "").
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
	}, map[int64]int64{})
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
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
	}, map[int64]int64{})
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
