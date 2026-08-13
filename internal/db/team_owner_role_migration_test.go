package db

import (
	"os"
	"strings"
	"testing"

	"mosona-manager/deploy/postgres"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

const teamOwnerRoleMigration = "migrations/20260814_normalize_team_owner_membership.sql"

func TestTeamOwnerRoleMigrationUpsertsAdministratorMembership(t *testing.T) {
	migration, err := postgres.Migrations.ReadFile(teamOwnerRoleMigration)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(migration)
	for _, fragment := range []string{
		"INSERT INTO m_team_user (team_id, user_id, role)",
		"SELECT id, owner_id, 0",
		"FROM teams",
		"ON CONFLICT (team_id, user_id) DO UPDATE",
		"SET role = 0",
		"WHERE m_team_user.role <> 0",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("owner role migration does not contain %q", fragment)
		}
	}
}

func TestTeamOwnerRoleMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("TEAM_OWNER_ROLE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEAM_OWNER_ROLE_TEST_DATABASE_URL is not set")
	}

	testDB, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	testDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = testDB.Close() })
	if _, err = testDB.Exec(`
		CREATE TEMP TABLE teams (
			id bigint PRIMARY KEY,
			owner_id bigint NOT NULL
		);
		CREATE TEMP TABLE m_team_user (
			team_id bigint NOT NULL,
			user_id bigint NOT NULL,
			role smallint NOT NULL,
			PRIMARY KEY (team_id, user_id)
		);
		INSERT INTO teams (id, owner_id) VALUES (1, 10), (2, 20), (3, 30);
		INSERT INTO m_team_user (team_id, user_id, role) VALUES
			(1, 10, 2),
			(2, 20, 0),
			(3, 31, 1);
	`); err != nil {
		t.Fatal(err)
	}

	migration, err := postgres.Migrations.ReadFile(teamOwnerRoleMigration)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = testDB.Exec(string(migration)); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}

	var invalidOwners int
	if err = testDB.Get(&invalidOwners, `
		SELECT COUNT(*)
		FROM teams AS t
		LEFT JOIN m_team_user AS mtu
		  ON mtu.team_id = t.id AND mtu.user_id = t.owner_id
		WHERE mtu.user_id IS NULL OR mtu.role <> 0
	`); err != nil {
		t.Fatal(err)
	}
	if invalidOwners != 0 {
		t.Fatalf("invalid owner memberships after migration = %d", invalidOwners)
	}

	var retainedRole int
	if err = testDB.Get(&retainedRole, "SELECT role FROM m_team_user WHERE team_id = 3 AND user_id = 31"); err != nil {
		t.Fatal(err)
	}
	if retainedRole != 1 {
		t.Fatalf("non-owner role = %d, want 1", retainedRole)
	}
}
