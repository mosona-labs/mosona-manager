package db

import (
	"sort"
	"strings"
	"testing"

	"mosona-manager/deploy/postgres"
)

const activeTeamMembershipMigration = "migrations/20260812_user_active_team_membership.sql"

func TestActiveTeamMembershipMigrationCleansOrphansAndReplacesForeignKey(t *testing.T) {
	migration, err := postgres.Migrations.ReadFile(activeTeamMembershipMigration)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(migration)

	required := []string{
		"DELETE FROM users_config AS uc",
		"NOT EXISTS",
		"mtu.team_id = uc.active_team",
		"mtu.user_id = uc.uid",
		"DROP CONSTRAINT IF EXISTS \"FK_UCT\"",
		"FOREIGN KEY (active_team, uid)",
		"REFERENCES m_team_user (team_id, user_id)",
		"ON DELETE CASCADE",
	}
	for _, fragment := range required {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("migration does not contain %q", fragment)
		}
	}

	if strings.Index(sqlText, "DELETE FROM users_config AS uc") > strings.Index(sqlText, "ADD CONSTRAINT \"FK_UCT\"") {
		t.Fatal("orphan user configurations must be removed before adding the membership foreign key")
	}
}

func TestActiveTeamMembershipMigrationSortsAfterSecureOIDC(t *testing.T) {
	migrations, err := postgres.Migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(migrations))
	for _, migration := range migrations {
		names = append(names, migration.Name())
	}
	sort.Strings(names)

	secureIndex := -1
	activeTeamIndex := -1
	for i, name := range names {
		switch name {
		case "20260812_secure_oidc_identity.sql":
			secureIndex = i
		case strings.TrimPrefix(activeTeamMembershipMigration, "migrations/"):
			activeTeamIndex = i
		}
	}
	if secureIndex == -1 || activeTeamIndex == -1 || activeTeamIndex <= secureIndex {
		t.Fatalf("migration order = %v; active-team migration must follow secure OIDC", names)
	}
}

func TestInitialSchemaUsesActiveTeamMembershipForeignKey(t *testing.T) {
	schema, err := postgres.InitSchema.ReadFile("init/001_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(schema)
	required := `FOREIGN KEY ("active_team", "uid") REFERENCES "m_team_user" ("team_id", "user_id") ON DELETE CASCADE`
	if !strings.Contains(sqlText, required) {
		t.Fatalf("initial schema does not contain %q", required)
	}
	if strings.Contains(sqlText, `FOREIGN KEY ("active_team") REFERENCES "teams"`) {
		t.Fatal("initial schema still permits active teams without a matching membership")
	}
}
