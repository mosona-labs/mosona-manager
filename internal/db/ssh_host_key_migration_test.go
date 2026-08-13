package db

import (
	"os"
	"strings"
	"testing"

	"mosona-manager/deploy/postgres"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func TestSSHHostKeyMigrationPinsWithoutTrustingLegacyRows(t *testing.T) {
	migration, err := postgres.Migrations.ReadFile("migrations/20260813_pin_ssh_host_keys.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(migration)

	required := []string{
		"ADD COLUMN IF NOT EXISTS host_key text",
		"CHECK (host_key IS NULL OR btrim(host_key) <> '')",
	}
	for _, fragment := range required {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("migration does not contain %q", fragment)
		}
	}
	if strings.Contains(sqlText, "UPDATE ssh") {
		t.Fatal("migration must not automatically trust host keys for legacy SSH records")
	}
}

func TestSSHHostKeyMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("SSH_HOST_KEY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SSH_HOST_KEY_TEST_DATABASE_URL is not set")
	}

	testDB, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	testDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = testDB.Close() })
	if _, err = testDB.Exec(`
		CREATE TEMP TABLE ssh (
			server_id bigint PRIMARY KEY
		);
		INSERT INTO ssh (server_id) VALUES (1);
	`); err != nil {
		t.Fatal(err)
	}

	migration, err := postgres.Migrations.ReadFile("migrations/20260813_pin_ssh_host_keys.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = testDB.Exec(string(migration)); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}

	var hostKey *string
	if err = testDB.Get(&hostKey, "SELECT host_key FROM ssh WHERE server_id = 1"); err != nil {
		t.Fatal(err)
	}
	if hostKey != nil {
		t.Fatalf("legacy host key = %q; expected NULL", *hostKey)
	}
	if _, err = testDB.Exec("UPDATE ssh SET host_key = '   ' WHERE server_id = 1"); err == nil {
		t.Fatal("blank SSH host key unexpectedly satisfied the database constraint")
	}
	if _, err = testDB.Exec("UPDATE ssh SET host_key = 'ssh-ed25519 AAAATEST' WHERE server_id = 1"); err != nil {
		t.Fatalf("nonblank SSH host key rejected: %v", err)
	}
}

func TestLegacySSHHostKeyCompatibilityMigration(t *testing.T) {
	migration, err := postgres.Migrations.ReadFile("migrations/20260814_allow_legacy_ssh_host_keys.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(migration)
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS trust_legacy_host_key boolean NOT NULL DEFAULT false",
		"UPDATE ssh\nSET trust_legacy_host_key = true\nWHERE host_key IS NULL",
		"CHECK ((host_key IS NULL) = trust_legacy_host_key)",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("compatibility migration does not contain %q", fragment)
		}
	}
}

func TestLegacySSHHostKeyCompatibilityMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("SSH_HOST_KEY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SSH_HOST_KEY_TEST_DATABASE_URL is not set")
	}

	testDB, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	testDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = testDB.Close() })
	if _, err = testDB.Exec(`
		CREATE TEMP TABLE ssh (
			server_id bigint PRIMARY KEY,
			host_key text
		);
		INSERT INTO ssh (server_id, host_key) VALUES
			(1, NULL),
			(2, 'ssh-ed25519 AAAAPINNED');
	`); err != nil {
		t.Fatal(err)
	}

	migration, err := postgres.Migrations.ReadFile("migrations/20260814_allow_legacy_ssh_host_keys.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = testDB.Exec(string(migration)); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}

	rows := []struct {
		ServerID           int64   `db:"server_id"`
		HostKey            *string `db:"host_key"`
		TrustLegacyHostKey bool    `db:"trust_legacy_host_key"`
	}{}
	if err = testDB.Select(&rows, "SELECT server_id, host_key, trust_legacy_host_key FROM ssh ORDER BY server_id"); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || !rows[0].TrustLegacyHostKey || rows[1].TrustLegacyHostKey {
		t.Fatalf("unexpected migrated trust states: %+v", rows)
	}

	if _, err = testDB.Exec("INSERT INTO ssh (server_id, host_key) VALUES (3, NULL)"); err == nil {
		t.Fatal("new unpinned SSH row unexpectedly satisfied strict trust-state constraint")
	}
	if _, err = testDB.Exec("UPDATE ssh SET host_key = 'ssh-ed25519 AAAAPIN', trust_legacy_host_key = false WHERE server_id = 1"); err != nil {
		t.Fatalf("pinning legacy SSH row failed: %v", err)
	}
}

func TestInitialSchemaIncludesNullableSSHHostKey(t *testing.T) {
	schema, err := postgres.InitSchema.ReadFile("init/001_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(schema)
	if !strings.Contains(sqlText, `"host_key" text`) {
		t.Fatal("initial schema does not include SSH host_key")
	}
	if strings.Contains(sqlText, `"host_key" text COLLATE "pg_catalog"."default" NOT NULL`) {
		t.Fatal("legacy SSH records must remain untrusted rather than receiving an implicit key")
	}
	if !strings.Contains(sqlText, `"trust_legacy_host_key" bool NOT NULL DEFAULT false`) {
		t.Fatal("initial schema does not default new SSH records to strict host-key validation")
	}
	if !strings.Contains(sqlText, `CHECK (("host_key" IS NULL) = "trust_legacy_host_key")`) {
		t.Fatal("initial schema does not enforce SSH host-key trust state")
	}
}
