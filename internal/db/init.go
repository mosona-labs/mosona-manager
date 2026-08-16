package db

import (
	"fmt"
	"io/fs"
	"log"
	"mosona-manager/deploy/postgres"
	"mosona-manager/internal/config"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var Db *sqlx.DB

const postgresApplicationName = "mosona-manager-hub"

func Init() {
	dsn := postgresDSN(
		config.Conf.PostgresHost,
		config.Conf.PostgresPort,
		config.Conf.PostgresUser,
		config.Conf.PostgresPass,
		config.Conf.PostgresDB,
		config.Conf.PostgresIdleTransactionTimeout,
	)

	var err error
	Db, err = sqlx.Open("postgres", dsn)
	if err != nil {
		log.Fatalln("Failed to connect to Postgres:", err)
	}

	Db.SetMaxOpenConns(50)
	Db.SetMaxIdleConns(25)
	Db.SetConnMaxLifetime(30 * time.Minute)

	for i := 0; i < 5; i++ {
		if err = Db.Ping(); err == nil {
			if err = runMigrations(); err != nil {
				log.Fatalln("Failed to run Postgres migrations:", err)
			}
			return
		}
		log.Printf("Postgres ping failed (attempt %d): %v", i+1, err)
		time.Sleep(time.Duration(i+1) * time.Second)
	}

	log.Fatalln("Failed to connect to Postgres after retries:", err)
}

func postgresDSN(host string, port int, user, password, database string, idleTransactionTimeout time.Duration) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable application_name=%s idle_in_transaction_session_timeout=%d",
		quoteDSNValue(host),
		port,
		quoteDSNValue(user),
		quoteDSNValue(password),
		quoteDSNValue(database),
		postgresApplicationName,
		idleTransactionTimeout.Milliseconds(),
	)
}

// quoteDSNValue quotes a keyword/value DSN value for lib/pq: values are
// wrapped in single quotes with backslashes and single quotes escaped, so
// whitespace or special characters in credentials cannot break parsing.
func quoteDSNValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return "'" + value + "'"
}

func runMigrations() error {
	initialSchemaApplied, err := ensureInitialSchema()
	if err != nil {
		return err
	}

	if _, err := Db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
  version varchar(255) PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		return err
	}

	migrations, err := fs.Glob(postgres.Migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(migrations)

	for _, migration := range migrations {
		version := path.Base(migration)

		if initialSchemaApplied {
			if _, err := Db.Exec("INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING", version); err != nil {
				return fmt.Errorf("record baseline migration %s: %w", version, err)
			}
			continue
		}

		var applied bool
		if err := Db.Get(&applied, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", version); err != nil {
			return err
		}
		if applied {
			continue
		}

		sqlBytes, err := postgres.Migrations.ReadFile(migration)
		if err != nil {
			return err
		}

		if err = applyMigration(version, sqlBytes); err != nil {
			return err
		}

		log.Printf("Applied Postgres migration: %s", version)
	}

	return nil
}

func applyMigration(version string, sqlBytes []byte) error {
	tx, err := Db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("apply migration %s: %w", version, err)
	}
	if _, err = tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}
	return nil
}

func ensureInitialSchema() (bool, error) {
	var schemaExists bool
	if err := Db.Get(&schemaExists, "SELECT to_regclass('public.users') IS NOT NULL"); err != nil {
		return false, err
	}
	if schemaExists {
		return false, nil
	}

	var tableCount int
	if err := Db.Get(&tableCount, `
SELECT count(*)
FROM information_schema.tables
WHERE table_schema = 'public'
  AND table_type = 'BASE TABLE'`); err != nil {
		return false, err
	}
	if tableCount > 0 {
		return false, fmt.Errorf("public schema is not empty but users table is missing")
	}

	schemaSQL, err := postgres.InitSchema.ReadFile("init/001_schema.sql")
	if err != nil {
		return false, err
	}

	if _, err = Db.Exec(string(schemaSQL)); err != nil {
		return false, fmt.Errorf("apply initial schema: %w", err)
	}

	log.Println("Applied Postgres initial schema")
	return true, nil
}
