//go:build integration

package db

import (
	"strings"
	"testing"
	"time"

	"mosona-manager/internal/config"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// TestInitAgainstRealPostgres verifies, against a real Postgres server, that
// Init applies the DSN session settings (application_name and
// idle_in_transaction_session_timeout), that migrations create the core
// tables, and that the server actually kills sessions idle in transaction.
//
// Requires a Postgres reachable at 127.0.0.1:55432 with user/password/db
// mm/mm/mm_db (e.g. docker run -p 55432:5432 postgres:16-alpine).
func TestInitAgainstRealPostgres(t *testing.T) {
	config.Conf.PostgresHost = "127.0.0.1"
	config.Conf.PostgresPort = 55432
	config.Conf.PostgresUser = "mm"
	config.Conf.PostgresPass = "mm"
	config.Conf.PostgresDB = "mm_db"
	config.Conf.PostgresIdleTransactionTimeout = 2 * time.Second

	Init()
	t.Cleanup(func() { _ = Db.Close() })

	t.Run("idle_in_transaction_session_timeout applied from DSN", func(t *testing.T) {
		var setting, unit string
		if err := Db.QueryRow(
			"SELECT setting, unit FROM pg_settings WHERE name = 'idle_in_transaction_session_timeout'",
		).Scan(&setting, &unit); err != nil {
			t.Fatalf("read pg_settings: %v", err)
		}
		if setting != "2000" || unit != "ms" {
			t.Fatalf("idle_in_transaction_session_timeout = %s%s, want 2000ms", setting, unit)
		}
	})

	t.Run("application_name visible in pg_stat_activity", func(t *testing.T) {
		var appName string
		if err := Db.QueryRow(
			"SELECT application_name FROM pg_stat_activity WHERE pid = pg_backend_pid()",
		).Scan(&appName); err != nil {
			t.Fatalf("read pg_stat_activity: %v", err)
		}
		if appName != postgresApplicationName {
			t.Fatalf("application_name = %q, want %q", appName, postgresApplicationName)
		}
	})

	t.Run("migrations created core tables", func(t *testing.T) {
		for _, table := range []string{"servers", "server_info", "server_alerts"} {
			var exists bool
			if err := Db.Get(&exists, "SELECT to_regclass($1) IS NOT NULL", "public."+table); err != nil {
				t.Fatalf("check table %s: %v", table, err)
			}
			if !exists {
				t.Fatalf("table public.%s does not exist after Init", table)
			}
		}
	})

	t.Run("server terminates session idle in transaction past 2s", func(t *testing.T) {
		pool, err := sqlx.Open("postgres", postgresDSN(
			config.Conf.PostgresHost,
			config.Conf.PostgresPort,
			config.Conf.PostgresUser,
			config.Conf.PostgresPass,
			config.Conf.PostgresDB,
			config.Conf.PostgresIdleTransactionTimeout,
		))
		if err != nil {
			t.Fatalf("open pool: %v", err)
		}
		pool.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = pool.Close() })

		conn, err := pool.Connx(t.Context())
		if err != nil {
			t.Fatalf("grab connection: %v", err)
		}
		defer func() { _ = conn.Close() }()

		if _, err = conn.ExecContext(t.Context(), "BEGIN"); err != nil {
			t.Fatalf("begin: %v", err)
		}

		var pid int
		if err = conn.QueryRowContext(t.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			t.Fatalf("read backend pid: %v", err)
		}

		var state string
		if err = Db.QueryRow(
			"SELECT state FROM pg_stat_activity WHERE pid = $1", pid,
		).Scan(&state); err != nil {
			t.Fatalf("read idle session state: %v", err)
		}
		if state != "idle in transaction" {
			t.Fatalf("idle session state = %q, want %q", state, "idle in transaction")
		}

		// Sit idle in the open transaction well past the 2s timeout.
		time.Sleep(3500 * time.Millisecond)

		var one int
		err = conn.QueryRowContext(t.Context(), "SELECT 1").Scan(&one)
		if err == nil {
			t.Fatal("query on session idle in transaction past 2s succeeded; want the server to terminate it")
		}
		t.Logf("idle session terminated as expected: %v", err)
		if !strings.Contains(err.Error(), "idle-in-transaction") && !strings.Contains(err.Error(), "bad connection") {
			t.Logf("warning: unexpected error text %q (still proves the session was killed)", err)
		}

		var backends int
		if err = Db.Get(&backends, "SELECT count(*) FROM pg_stat_activity WHERE pid = $1", pid); err != nil {
			t.Fatalf("check terminated backend: %v", err)
		}
		if backends != 0 {
			t.Fatalf("backend pid %d still present in pg_stat_activity after timeout", pid)
		}
	})
}
