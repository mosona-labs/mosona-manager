package db

import (
	"strings"
	"testing"
	"time"
)

func TestPostgresDSNIdentifiesApplicationAndLimitsIdleTransactions(t *testing.T) {
	dsn := postgresDSN("postgres", 5432, "user", "password", "database", 90*time.Second)
	for _, expected := range []string{
		"host='postgres'",
		"user='user'",
		"password='password'",
		"dbname='database'",
		"application_name=mosona-manager-hub",
		"idle_in_transaction_session_timeout=90000",
	} {
		if !strings.Contains(dsn, expected) {
			t.Fatalf("postgresDSN() = %q, missing %q", dsn, expected)
		}
	}
}

func TestPostgresDSNQuotesSpecialCharacters(t *testing.T) {
	dsn := postgresDSN("my host", 5432, `us'er`, `pa\ss word`, "database", 90*time.Second)
	for _, expected := range []string{
		`host='my host'`,
		`user='us\'er'`,
		`password='pa\\ss word'`,
	} {
		if !strings.Contains(dsn, expected) {
			t.Fatalf("postgresDSN() = %q, missing %q", dsn, expected)
		}
	}
}

func TestPostgresDSNDisablesIdleTransactionTimeoutAtZero(t *testing.T) {
	dsn := postgresDSN("postgres", 5432, "user", "password", "database", 0)
	if !strings.Contains(dsn, "idle_in_transaction_session_timeout=0") {
		t.Fatalf("postgresDSN() = %q, missing idle_in_transaction_session_timeout=0", dsn)
	}
}
