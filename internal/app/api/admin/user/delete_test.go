package muser

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mosona-manager/internal/db"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

func TestDeleteUserReturnsOwnedTeamsConflict(t *testing.T) {
	mock := setDeleteUserMockDB(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT username FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("target"))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "Production"))
	mock.ExpectRollback()

	e := echo.New()
	e.DELETE("/users/:id", del)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/users/42?confirm=target", nil))

	body := recorder.Body.String()
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, body)
	}
	for _, fragment := range []string{
		`"code":"user_owns_teams"`,
		`"teams":[{"id":7,"name":"Production"}]`,
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("response does not contain %q: %s", fragment, body)
		}
	}
}

func TestDeleteUserRequiresConfirmation(t *testing.T) {
	mock := setDeleteUserMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT username FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("target"))
	mock.ExpectRollback()

	e := echo.New()
	e.DELETE("/users/:id", del)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/users/42", nil))

	body := recorder.Body.String()
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, body)
	}
	if !strings.Contains(body, `"code":"delete_confirmation_mismatch"`) {
		t.Fatalf("unexpected response: %s", body)
	}
}

func setDeleteUserMockDB(t *testing.T) sqlmock.Sqlmock {
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
