package db

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"mosona-manager/internal/config"
)

func setupSyncConfigTest(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	oldDB := Db
	oldConfig := config.ReadDynamicConf()
	Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		config.ReplaceDynamicConf(oldConfig)
		Db = oldDB
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})
	return mock
}

func TestSyncConfigPublishesCompleteSnapshot(t *testing.T) {
	mock := setupSyncConfigTest(t)
	config.ReplaceDynamicConf(config.DynamicConfigType{
		Title:         "old-title",
		Domain:        "https://old.example.com",
		Token:         "old-token",
		SMTPPort:      25,
		SessionBindIP: true,
		TrustProxy:    true,
	})
	mock.ExpectQuery(regexp.QuoteMeta("SELECT key, value FROM config")).WillReturnRows(
		sqlmock.NewRows([]string{"key", "value"}).
			AddRow("title", "new-title").
			AddRow("domain", "https://new.example.com").
			AddRow("token", "new-token").
			AddRow("smtp_port", "587").
			AddRow("session_bind_ip", "false"),
	)

	if err := SyncConfig(); err != nil {
		t.Fatal(err)
	}
	got := config.ReadDynamicConf()
	if got.Title != "new-title" || got.Domain != "https://new.example.com" || got.Token != "new-token" || got.SMTPPort != 587 {
		t.Fatalf("unexpected published snapshot: %+v", got)
	}
	if got.SessionBindIP {
		t.Fatal("expected session_bind_ip=false")
	}
	if !got.TrustProxy {
		t.Fatal("missing database row should preserve the previous value")
	}
}

func TestSyncConfigDoesNotPublishWhenTokenPersistenceFails(t *testing.T) {
	mock := setupSyncConfigTest(t)
	previous := config.DynamicConfigType{
		Title:  "stable-title",
		Domain: "https://stable.example.com",
		Token:  "",
	}
	config.ReplaceDynamicConf(previous)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT key, value FROM config")).WillReturnRows(
		sqlmock.NewRows([]string{"key", "value"}).
			AddRow("title", "unpublished-title").
			AddRow("domain", "https://unpublished.example.com"),
	)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO config (key, value)
		 VALUES ($1, $2)
		 ON CONFLICT (key) 
		 DO UPDATE SET 
		 	value = EXCLUDED.value`)).
		WithArgs("token", sqlmock.AnyArg()).
		WillReturnError(errors.New("write failed"))

	if err := SyncConfig(); err == nil {
		t.Fatal("expected token persistence failure")
	}
	if got := config.ReadDynamicConf(); got != previous {
		t.Fatalf("configuration changed after failed reload: got %+v, want %+v", got, previous)
	}
}

func TestSyncConfigDoesNotPublishWhenTokenGenerationFails(t *testing.T) {
	mock := setupSyncConfigTest(t)
	previous := config.DynamicConfigType{
		Title:  "stable-title",
		Domain: "https://stable.example.com",
		Token:  "",
	}
	config.ReplaceDynamicConf(previous)
	oldGenerator := generateConfigToken
	generateConfigToken = func(int) string { return "" }
	t.Cleanup(func() { generateConfigToken = oldGenerator })
	mock.ExpectQuery(regexp.QuoteMeta("SELECT key, value FROM config")).WillReturnRows(
		sqlmock.NewRows([]string{"key", "value"}).
			AddRow("title", "unpublished-title").
			AddRow("domain", "https://unpublished.example.com"),
	)

	if err := SyncConfig(); err == nil {
		t.Fatal("expected token generation failure")
	}
	if got := config.ReadDynamicConf(); got != previous {
		t.Fatalf("configuration changed after failed reload: got %+v, want %+v", got, previous)
	}
}
