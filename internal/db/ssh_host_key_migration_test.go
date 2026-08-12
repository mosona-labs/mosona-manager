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
}
