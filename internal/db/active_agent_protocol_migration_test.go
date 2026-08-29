package db

import (
	"os"
	"strings"
	"testing"

	"mosona-manager/deploy/postgres"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func TestActiveAgentProtocolMigrationPreservesLegacyFleet(t *testing.T) {
	migration, err := postgres.Migrations.ReadFile("migrations/20260828_active_agent_protocol_version.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(migration)
	for _, want := range []string{
		"WHEN btrim(public_key) <> '' THEN 2",
		"ELSE 1",
		"ALTER COLUMN protocol_version SET DEFAULT 2",
		"CHECK (protocol_version IN (1, 2))",
		"CHECK (protocol_version <> 1 OR btrim(public_key) = '')",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration is missing %q", want)
		}
	}
}

func TestActiveAgentProtocolMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("ACTIVE_AGENT_PROTOCOL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ACTIVE_AGENT_PROTOCOL_TEST_DATABASE_URL is not set")
	}

	testDB, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	testDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = testDB.Close() })
	if _, err = testDB.Exec(`
		CREATE TEMP TABLE agents (
			server_id bigint PRIMARY KEY,
			public_key text NOT NULL DEFAULT ''
		);
		INSERT INTO agents (server_id, public_key) VALUES
			(1, ''),
			(2, '   '),
			(3, 'pinned-key');
	`); err != nil {
		t.Fatal(err)
	}

	migration, err := postgres.Migrations.ReadFile("migrations/20260828_active_agent_protocol_version.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = testDB.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	if _, err = testDB.Exec(string(migration)); err != nil {
		t.Fatalf("migration is not idempotent: %v", err)
	}

	var versions []int16
	if err = testDB.Select(&versions, "SELECT protocol_version FROM agents ORDER BY server_id"); err != nil {
		t.Fatal(err)
	}
	want := []int16{1, 1, 2}
	if len(versions) != len(want) {
		t.Fatalf("protocol versions = %v, want %v", versions, want)
	}
	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("protocol versions = %v, want %v", versions, want)
		}
	}

	if _, err = testDB.Exec("INSERT INTO agents (server_id, public_key) VALUES (4, '')"); err != nil {
		t.Fatalf("insert using v2 default: %v", err)
	}
	var defaultVersion int16
	if err = testDB.Get(&defaultVersion, "SELECT protocol_version FROM agents WHERE server_id = 4"); err != nil {
		t.Fatal(err)
	}
	if defaultVersion != 2 {
		t.Fatalf("default protocol version = %d, want 2", defaultVersion)
	}

	invalidWrites := []string{
		"INSERT INTO agents (server_id, public_key, protocol_version) VALUES (5, '', NULL)",
		"INSERT INTO agents (server_id, public_key, protocol_version) VALUES (6, '', 3)",
		"INSERT INTO agents (server_id, public_key, protocol_version) VALUES (7, 'pinned-key', 1)",
	}
	for _, query := range invalidWrites {
		if _, err = testDB.Exec(query); err == nil {
			t.Fatalf("invalid Active Agent protocol state was accepted: %s", query)
		}
	}
	if _, err = testDB.Exec("INSERT INTO agents (server_id, public_key, protocol_version) VALUES (8, '   ', 1)"); err != nil {
		t.Fatalf("legacy row with blank public key was rejected: %v", err)
	}
}
