package db

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"mosona-manager/deploy/postgres"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

const activeAgentPrivateKeyMigration = "migrations/20260829_encrypt_active_agent_private_keys.sql"

func TestActiveAgentPrivateKeyMigrationUsesBinaryStorage(t *testing.T) {
	migration, err := postgres.Migrations.ReadFile(activeAgentPrivateKeyMigration)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(migration)
	for _, fragment := range []string{
		"ALTER COLUMN private_key DROP DEFAULT",
		"ALTER COLUMN private_key TYPE bytea",
		"USING convert_to(private_key, 'UTF8')",
		"ALTER COLUMN private_key SET DEFAULT ''::bytea",
		"LOCK TABLE agents IN SHARE ROW EXCLUSIVE MODE",
		"WHERE agent_uid <> ''",
		"HAVING count(*) > 1",
		"USING ERRCODE = '23505'",
		`DROP INDEX IF EXISTS "IDX_AAU"`,
		`CREATE UNIQUE INDEX "IDX_AAU"`,
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("migration does not contain %q", fragment)
		}
	}

	schema, err := postgres.InitSchema.ReadFile("init/001_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), `"private_key" bytea NOT NULL DEFAULT ''::bytea`) {
		t.Fatal("fresh schema does not use bytea for Agent private keys")
	}
	if !strings.Contains(string(schema), `CREATE UNIQUE INDEX "IDX_AAU"`) ||
		!strings.Contains(string(schema), `) WHERE "agent_uid" <> '';`) {
		t.Fatal("fresh schema does not enforce unique non-empty Agent UIDs")
	}
}

func TestActiveAgentPrivateKeyMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AGENT_PRIVATE_KEY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AGENT_PRIVATE_KEY_TEST_DATABASE_URL is not set")
	}

	testDB, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	testDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = testDB.Close() })
	privateKey := []byte("-----BEGIN PRIVATE KEY-----\nlegacy-pem\n-----END PRIVATE KEY-----\n")
	if _, err = testDB.Exec(`
		CREATE TEMP TABLE agents (
			server_id bigint PRIMARY KEY,
			agent_uid char(36) NOT NULL DEFAULT '',
			private_key text NOT NULL DEFAULT ''
		);
		CREATE INDEX "IDX_AAU" ON agents (agent_uid);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err = testDB.Exec("INSERT INTO agents (server_id, agent_uid, private_key) VALUES (1, 'agent-one', $1)", privateKey); err != nil {
		t.Fatal(err)
	}

	migration, err := postgres.Migrations.ReadFile(activeAgentPrivateKeyMigration)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = testDB.Exec(string(migration)); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}

	var stored []byte
	if err = testDB.Get(&stored, "SELECT private_key FROM agents WHERE server_id = 1"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, privateKey) {
		t.Fatalf("private key bytes changed during migration: %q", stored)
	}
	var dataType string
	if err = testDB.Get(&dataType, `
		SELECT format_type(atttypid, atttypmod)
		FROM pg_attribute
		WHERE attrelid = 'agents'::regclass AND attname = 'private_key'
	`); err != nil {
		t.Fatal(err)
	}
	if dataType != "bytea" {
		t.Fatalf("private_key type = %q, want bytea", dataType)
	}
	if _, err = testDB.Exec("INSERT INTO agents (server_id) VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	if err = testDB.Get(&stored, "SELECT private_key FROM agents WHERE server_id = 2"); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("default private key = %x, want empty bytea", stored)
	}
	if _, err = testDB.Exec("INSERT INTO agents (server_id, agent_uid) VALUES (3, ''), (4, '')"); err != nil {
		t.Fatalf("unique index rejected empty Agent UIDs: %v", err)
	}
	if _, err = testDB.Exec("INSERT INTO agents (server_id, agent_uid) VALUES (5, 'agent-one')"); err == nil {
		t.Fatal("unique index accepted a duplicate non-empty Agent UID")
	}
	var unique bool
	if err = testDB.Get(&unique, `
		SELECT indisunique
		FROM pg_index
		WHERE indexrelid = '"IDX_AAU"'::regclass
	`); err != nil {
		t.Fatal(err)
	}
	if !unique {
		t.Fatal("IDX_AAU is not unique after migration")
	}

	if _, err = testDB.Exec(`
		DROP TABLE agents;
		CREATE TEMP TABLE agents (
			server_id bigint PRIMARY KEY,
			agent_uid char(36) NOT NULL DEFAULT '',
			private_key text NOT NULL DEFAULT ''
		);
		CREATE INDEX "IDX_AAU" ON agents (agent_uid);
		INSERT INTO agents (server_id, agent_uid, private_key) VALUES
			(10, 'duplicate-agent', 'first-key'),
			(11, 'duplicate-agent', 'second-key');
	`); err != nil {
		t.Fatal(err)
	}
	if _, err = testDB.Exec(string(migration)); err == nil {
		t.Fatal("migration accepted pre-existing duplicate non-empty Agent UIDs")
	}
	if err = testDB.Get(&dataType, `
		SELECT format_type(atttypid, atttypmod)
		FROM pg_attribute
		WHERE attrelid = 'agents'::regclass AND attname = 'private_key'
	`); err != nil {
		t.Fatal(err)
	}
	if dataType != "text" {
		t.Fatalf("failed duplicate migration left private_key type = %q, want rolled-back text", dataType)
	}
	if err = testDB.Get(&unique, `
		SELECT indisunique
		FROM pg_index
		WHERE indexrelid = '"IDX_AAU"'::regclass
	`); err != nil {
		t.Fatal(err)
	}
	if unique {
		t.Fatal("failed duplicate migration replaced the original non-unique index")
	}
}
