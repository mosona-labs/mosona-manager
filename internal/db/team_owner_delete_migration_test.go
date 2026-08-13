package db

import (
	"os"
	"strings"
	"testing"

	"mosona-manager/deploy/postgres"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func TestTeamOwnerDeleteMigrationReplacesCascadeWithRestrict(t *testing.T) {
	migration, err := postgres.Migrations.ReadFile("migrations/20260813_restrict_team_owner_delete.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(migration)

	for _, fragment := range []string{
		`DROP CONSTRAINT IF EXISTS "FK_TU"`,
		`ADD CONSTRAINT "FK_TU"`,
		`FOREIGN KEY (owner_id) REFERENCES users (id)`,
		`ON DELETE RESTRICT`,
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("migration does not contain %q", fragment)
		}
	}
	if strings.Contains(sqlText, "ON DELETE CASCADE") {
		t.Fatal("team owner migration still permits cascading team deletion")
	}
}

func TestInitialSchemaRestrictsTeamOwnerDeletion(t *testing.T) {
	schema, err := postgres.InitSchema.ReadFile("init/001_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(schema)
	required := `CONSTRAINT "FK_TU" FOREIGN KEY ("owner_id") REFERENCES "users" ("id") ON DELETE RESTRICT`
	if !strings.Contains(sqlText, required) {
		t.Fatalf("initial schema does not contain %q", required)
	}
}

func TestTeamOwnerDeleteMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("TEAM_OWNER_DELETE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEAM_OWNER_DELETE_TEST_DATABASE_URL is not set")
	}

	testDB, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	testDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = testDB.Close() })
	if _, err = testDB.Exec(`
		CREATE TEMP TABLE users (id bigint PRIMARY KEY);
		CREATE TEMP TABLE teams (
			id bigint PRIMARY KEY,
			owner_id bigint NOT NULL,
			CONSTRAINT "FK_TU" FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE CASCADE
		);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO teams (id, owner_id) VALUES (7, 1);
	`); err != nil {
		t.Fatal(err)
	}

	migration, err := postgres.Migrations.ReadFile("migrations/20260813_restrict_team_owner_delete.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = testDB.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	if _, err = testDB.Exec("DELETE FROM users WHERE id = 1"); err == nil {
		t.Fatal("owner deletion unexpectedly succeeded")
	}

	var count int
	if err = testDB.Get(&count, "SELECT COUNT(*) FROM teams WHERE id = 7"); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("team count = %d, want 1", count)
	}
}
