package db

import (
	"os"
	"strings"
	"testing"

	"mosona-manager/deploy/postgres"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

const alertConfigBoundsMigration = "migrations/20260814_alert_config_bounds.sql"

func TestAlertConfigBoundsMigrationRepairsRowsBeforeAddingConstraints(t *testing.T) {
	migration, err := postgres.Migrations.ReadFile(alertConfigBoundsMigration)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(migration)
	for _, fragment := range []string{
		"UPDATE server_alerts",
		"UPDATE team_alerts",
		"LEAST(GREATEST(threshold, 1), 100)",
		"LEAST(GREATEST(for_duration, 1), 1440)",
		"ADD CONSTRAINT server_alerts_config_bounds CHECK",
		"ADD CONSTRAINT team_alerts_config_bounds CHECK",
		"item = 'expiry_reminder' AND threshold BETWEEN 1 AND 7 AND for_duration = 0",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("migration does not contain %q", fragment)
		}
	}
	if strings.Index(sqlText, "UPDATE server_alerts") > strings.Index(sqlText, "ADD CONSTRAINT server_alerts_config_bounds") {
		t.Fatal("server alert rows must be repaired before adding the constraint")
	}
	if strings.Index(sqlText, "UPDATE team_alerts") > strings.Index(sqlText, "ADD CONSTRAINT team_alerts_config_bounds") {
		t.Fatal("team alert rows must be repaired before adding the constraint")
	}
}

func TestInitialSchemaConstrainsAlertConfiguration(t *testing.T) {
	schema, err := postgres.InitSchema.ReadFile("init/001_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(schema)
	for _, fragment := range []string{
		`CONSTRAINT "server_alerts_config_bounds" CHECK`,
		`CONSTRAINT "team_alerts_config_bounds" CHECK`,
		`"item" = 'status' AND "threshold" = 0 AND "for_duration" BETWEEN 1 AND 1440`,
		`"item" = 'expiry_reminder' AND "threshold" BETWEEN 1 AND 7 AND "for_duration" = 0`,
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("initial schema does not contain %q", fragment)
		}
	}
}

func TestAlertConfigBoundsMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("ALERT_CONFIG_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ALERT_CONFIG_TEST_DATABASE_URL is not set")
	}

	testDB, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	testDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = testDB.Close() })
	if _, err = testDB.Exec(`
		CREATE TEMP TABLE server_alerts (item text NOT NULL, threshold integer NOT NULL, for_duration integer NOT NULL);
		CREATE TEMP TABLE team_alerts (item text NOT NULL, threshold integer NOT NULL, for_duration integer NOT NULL);
		INSERT INTO server_alerts VALUES ('cpu_usage', -5, 999999), ('status', 42, -1);
		INSERT INTO team_alerts VALUES ('expiry_reminder', 100, 30), ('write_iops', 2000000, 0);
	`); err != nil {
		t.Fatal(err)
	}

	migration, err := postgres.Migrations.ReadFile(alertConfigBoundsMigration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = testDB.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}

	invalidWrites := []string{
		"INSERT INTO server_alerts VALUES ('cpu_usage', 0, 10)",
		"INSERT INTO server_alerts VALUES ('status', 0, 1441)",
		"INSERT INTO team_alerts VALUES ('expiry_reminder', 8, 0)",
		"INSERT INTO team_alerts VALUES ('write_iops', 100, -1)",
	}
	for _, query := range invalidWrites {
		if _, err = testDB.Exec(query); err == nil {
			t.Fatalf("invalid alert configuration satisfied CHECK: %s", query)
		}
	}

	var count int
	if err = testDB.Get(&count, `
		SELECT COUNT(*) FROM server_alerts
		WHERE (item = 'cpu_usage' AND threshold = 1 AND for_duration = 1440)
		   OR (item = 'status' AND threshold = 0 AND for_duration = 1)
	`); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("repaired server alert count = %d, want 2", count)
	}
}
