package muser

import (
	"mosona-manager/internal/db"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

const (
	userSearchListSQL         = `SELECT id, username, email, is_admin, verified, (CASE WHEN totp IS NULL THEN false ELSE true END) AS totp_enabled, created_at, updated_at, login_at FROM users WHERE (username LIKE $1 ESCAPE '\' OR email LIKE $2 ESCAPE '\') ORDER BY id DESC LIMIT 10 OFFSET 10`
	userSearchCountSQL        = `SELECT COUNT(id) FROM users WHERE (username LIKE $1 ESCAPE '\' OR email LIKE $2 ESCAPE '\')`
	userNumericSearchListSQL  = `SELECT id, username, email, is_admin, verified, (CASE WHEN totp IS NULL THEN false ELSE true END) AS totp_enabled, created_at, updated_at, login_at FROM users WHERE (id = $1 OR username LIKE $2 ESCAPE '\' OR email LIKE $3 ESCAPE '\') ORDER BY id DESC LIMIT 20 OFFSET 0`
	userNumericSearchCountSQL = `SELECT COUNT(id) FROM users WHERE (id = $1 OR username LIKE $2 ESCAPE '\' OR email LIKE $3 ESCAPE '\')`
)

func useUserListMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	previousDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		db.Db = previousDB
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})
	return mock
}

func serveUserList(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	recorder := httptest.NewRecorder()
	c := e.NewContext(request, recorder)
	if err := list(c); err != nil {
		t.Fatal(err)
	}
	return recorder
}

func TestListNonNumericSearchDoesNotCompareIDToText(t *testing.T) {
	mock := useUserListMockDB(t)
	pattern := `%alice\_\%%`
	createdAt := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "username", "email", "is_admin", "verified", "totp_enabled", "created_at", "updated_at", "login_at",
	}).AddRow(int64(7), "alice_%", "alice@example.com", false, true, false, createdAt, createdAt, nil)
	mock.ExpectQuery(userSearchListSQL).WithArgs(pattern, pattern).WillReturnRows(rows)
	mock.ExpectQuery(userSearchCountSQL).WithArgs(pattern, pattern).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	recorder := serveUserList(t, "/?page=2&size=10&search=alice_%25")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"total":1`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestListRejectsInvalidPagination(t *testing.T) {
	recorder := serveUserList(t, "/?page=0&size=20")
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"msg":"Invalid pagination"`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestListNumericSearchIncludesIDCondition(t *testing.T) {
	mock := useUserListMockDB(t)
	pattern := "%123%"
	createdAt := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "username", "email", "is_admin", "verified", "totp_enabled", "created_at", "updated_at", "login_at",
	}).AddRow(int64(123), "numeric", "numeric@example.com", false, true, false, createdAt, createdAt, nil)
	mock.ExpectQuery(userNumericSearchListSQL).
		WithArgs(int64(123), pattern, pattern).
		WillReturnRows(rows)
	mock.ExpectQuery(userNumericSearchCountSQL).
		WithArgs(int64(123), pattern, pattern).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	recorder := serveUserList(t, "/?search=123")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"total":1`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
