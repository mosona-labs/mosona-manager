package init

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mosona-manager/internal/db"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

func TestInitializeSystemLocksAndRechecksDatabaseBeforeCreatingAdmin(t *testing.T) {
	mock := setInitializationMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).
		WithArgs(initializationLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT EXISTS.*FROM config WHERE key = 'init' AND value = 'true'`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`INSERT INTO users \(username, email, password, salt, verified, is_admin\)`).
		WithArgs("admin", "admin@example.com", sqlmock.AnyArg(), sqlmock.AnyArg(), true, true).
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectInitializationConfig(mock, "registration_enabled", "false")
	expectInitializationConfig(mock, "domain", "https://example.com")
	expectInitializationConfig(mock, "init", "true")
	mock.ExpectCommit()

	if err := initializeSystem(
		context.Background(),
		"admin", "admin@example.com", "Correct-Horse-1!", "https://example.com", "false",
	); err != nil {
		t.Fatal(err)
	}
}

func TestInitializeSystemRejectsAlreadyInitializedDatabase(t *testing.T) {
	mock := setInitializationMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).
		WithArgs(initializationLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT EXISTS.*FROM config WHERE key = 'init' AND value = 'true'`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	err := initializeSystem(
		context.Background(),
		"attacker", "attacker@example.com", "Password-1!", "https://attacker.example", "true",
	)
	if !errors.Is(err, errAlreadyInitialized) {
		t.Fatalf("initializeSystem() error = %v, want errAlreadyInitialized", err)
	}
}

func TestInitializeSystemRollsBackWhenLockFails(t *testing.T) {
	mock := setInitializationMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).
		WithArgs(initializationLockID).
		WillReturnError(errors.New("lock failed"))
	mock.ExpectRollback()

	err := initializeSystem(
		context.Background(),
		"admin", "admin@example.com", "Password-1!", "https://example.com", "false",
	)
	if err == nil || err.Error() != "lock failed" {
		t.Fatalf("initializeSystem() error = %v, want lock failure", err)
	}
}

func TestInitializeSystemRollsBackWhenDatabaseRecheckFails(t *testing.T) {
	mock := setInitializationMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).
		WithArgs(initializationLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT EXISTS.*FROM config WHERE key = 'init' AND value = 'true'`).
		WillReturnError(errors.New("recheck failed"))
	mock.ExpectRollback()

	err := initializeSystem(
		context.Background(),
		"admin", "admin@example.com", "Password-1!", "https://example.com", "false",
	)
	if err == nil || err.Error() != "recheck failed" {
		t.Fatalf("initializeSystem() error = %v, want recheck failure", err)
	}
}

func TestInitializeReturnsConflictWhenDatabaseIsAlreadyInitialized(t *testing.T) {
	mock := setInitializationMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).
		WithArgs(initializationLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT EXISTS.*FROM config WHERE key = 'init' AND value = 'true'`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	form := url.Values{
		"username":            {"attacker"},
		"email":               {"attacker@example.com"},
		"password":            {"Password-1!"},
		"website_url":         {"https://attacker.example"},
		"registration_enable": {"true"},
	}
	e := echo.New()
	e.POST("/init", initialize)
	request := httptest.NewRequest(http.MethodPost, "/init", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"already_initialized"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func expectInitializationConfig(mock sqlmock.Sqlmock, key, value string) {
	mock.ExpectExec(`(?s)INSERT INTO config \(key, value\).*ON CONFLICT \(key\) DO UPDATE SET value = \$2`).
		WithArgs(key, value).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func setInitializationMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		db.Db = oldDB
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})
	return mock
}
