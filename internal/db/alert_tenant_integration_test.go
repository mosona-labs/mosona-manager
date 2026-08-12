package db

import (
	"errors"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func TestUpsertServerAlertPostgresTenantIsolation(t *testing.T) {
	dsn := os.Getenv("ALERT_TENANT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ALERT_TENANT_TEST_DATABASE_URL is not set")
	}

	testDB, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	testDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = testDB.Close() })
	if _, err = testDB.Exec(`
		CREATE TEMP TABLE servers (
			id bigint PRIMARY KEY,
			team_id bigint NOT NULL
		);
		CREATE TEMP TABLE server_alerts (
			id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			server_id bigint NOT NULL REFERENCES servers(id),
			item text NOT NULL,
			threshold integer NOT NULL,
			for_duration integer NOT NULL,
			UNIQUE (server_id, item)
		);
		INSERT INTO servers (id, team_id) VALUES (11, 1), (22, 2);
	`); err != nil {
		t.Fatal(err)
	}

	oldDB := Db
	Db = testDB
	t.Cleanup(func() { Db = oldDB })

	if err = UpsertServerAlert(1, 11, "cpu_usage", 70, 5); err != nil {
		t.Fatalf("owned first insert: %v", err)
	}
	if err = UpsertServerAlert(1, 22, "cpu_usage", 99, 9); !errors.Is(err, ErrAlertServerNotFound) {
		t.Fatalf("cross-team first insert error = %v", err)
	}
	if err = UpsertServerAlert(2, 22, "cpu_usage", 60, 4); err != nil {
		t.Fatalf("victim first insert: %v", err)
	}
	if err = UpsertServerAlert(1, 22, "cpu_usage", 99, 9); !errors.Is(err, ErrAlertServerNotFound) {
		t.Fatalf("cross-team conflict update error = %v", err)
	}

	var threshold, forDuration int
	if err = testDB.QueryRow(`
		SELECT threshold, for_duration
		FROM server_alerts
		WHERE server_id = 22 AND item = 'cpu_usage'
	`).Scan(&threshold, &forDuration); err != nil {
		t.Fatal(err)
	}
	if threshold != 60 || forDuration != 4 {
		t.Fatalf("victim alert changed to threshold=%d duration=%d", threshold, forDuration)
	}

	if err = UpsertServerAlert(2, 22, "cpu_usage", 75, 6); err != nil {
		t.Fatalf("owned conflict update: %v", err)
	}
	if err = testDB.QueryRow(`
		SELECT threshold, for_duration
		FROM server_alerts
		WHERE server_id = 22 AND item = 'cpu_usage'
	`).Scan(&threshold, &forDuration); err != nil {
		t.Fatal(err)
	}
	if threshold != 75 || forDuration != 6 {
		t.Fatalf("owned alert update = threshold=%d duration=%d", threshold, forDuration)
	}
}
