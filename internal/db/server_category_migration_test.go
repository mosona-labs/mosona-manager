package db

import (
	"os"
	"strings"
	"testing"

	"mosona-manager/deploy/postgres"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

const serverCategoryTenantMigration = "migrations/20260813_server_category_tenant.sql"

func TestServerCategoryTenantMigrationRepairsRowsAndAddsCompositeForeignKey(t *testing.T) {
	migration, err := postgres.Migrations.ReadFile(serverCategoryTenantMigration)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(migration)

	for _, fragment := range []string{
		"UPDATE servers AS s",
		"WHERE c.team = s.team_id",
		"DROP CONSTRAINT IF EXISTS \"FK_SC\"",
		"UNIQUE (team, id)",
		"FOREIGN KEY (team_id, category) REFERENCES categories (team, id)",
		"ON DELETE RESTRICT",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("migration does not contain %q", fragment)
		}
	}
	if strings.Index(sqlText, "UPDATE servers AS s") > strings.Index(sqlText, "ADD CONSTRAINT \"FK_SC\"") {
		t.Fatal("cross-team server categories must be repaired before adding the composite foreign key")
	}
}

func TestInitialSchemaUsesTenantScopedServerCategoryForeignKey(t *testing.T) {
	schema, err := postgres.InitSchema.ReadFile("init/001_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(schema)
	for _, fragment := range []string{
		`CONSTRAINT "categories_team_id_key" UNIQUE ("team", "id")`,
		`CONSTRAINT "FK_SC" FOREIGN KEY ("team_id", "category") REFERENCES "categories" ("team", "id")`,
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("initial schema does not contain %q", fragment)
		}
	}
	if strings.Contains(sqlText, `CONSTRAINT "FK_SC" FOREIGN KEY ("category")`) {
		t.Fatal("initial schema still allows a server to reference another team's category")
	}
}

func TestServerCategoryTenantMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("SERVER_CATEGORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SERVER_CATEGORY_TEST_DATABASE_URL is not set")
	}

	testDB, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	testDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = testDB.Close() })
	if _, err = testDB.Exec(`
		CREATE TEMP TABLE categories (
			id bigint PRIMARY KEY,
			team bigint NOT NULL,
			sort integer NOT NULL DEFAULT 0
		);
		CREATE TEMP TABLE servers (
			id bigint PRIMARY KEY,
			team_id bigint NOT NULL,
			category bigint NOT NULL,
			CONSTRAINT "FK_SC" FOREIGN KEY (category) REFERENCES categories (id) ON DELETE RESTRICT
		);
		INSERT INTO categories (id, team, sort) VALUES (11, 1, 0), (12, 1, 1), (22, 2, 0);
		INSERT INTO servers (id, team_id, category) VALUES (91, 1, 22);
	`); err != nil {
		t.Fatal(err)
	}

	migration, err := postgres.Migrations.ReadFile(serverCategoryTenantMigration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = testDB.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}

	var categoryID int64
	if err = testDB.Get(&categoryID, "SELECT category FROM servers WHERE id = 91"); err != nil {
		t.Fatal(err)
	}
	if categoryID != 11 {
		t.Fatalf("repaired category = %d, want 11", categoryID)
	}
	if _, err = testDB.Exec("UPDATE servers SET category = 22 WHERE id = 91"); err == nil {
		t.Fatal("cross-team category update unexpectedly satisfied the composite foreign key")
	}
}
