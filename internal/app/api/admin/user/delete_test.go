package muser

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

func TestDeleteUserReturnsOwnedTeamsConflict(t *testing.T) {
	mock := setDeleteUserMockDB(t)

	expectSuccessfulReauthentication(mock, 1, "admin-password")
	mock.ExpectBegin()
	expectLockedAdmins(mock, 1)
	mock.ExpectQuery(`SELECT username, is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "is_admin"}).AddRow("target", false))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "Production"))
	mock.ExpectRollback()

	recorder := httptest.NewRecorder()
	serveDeleteRequest(recorder, 1, "/users/42?confirm=target", url.Values{
		"current_password": {"admin-password"},
	})

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
	expectSuccessfulReauthentication(mock, 1, "admin-password")
	mock.ExpectBegin()
	expectLockedAdmins(mock, 1)
	mock.ExpectQuery(`SELECT username, is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "is_admin"}).AddRow("target", false))
	mock.ExpectRollback()

	recorder := httptest.NewRecorder()
	serveDeleteRequest(recorder, 1, "/users/42", url.Values{
		"current_password": {"admin-password"},
	})

	body := recorder.Body.String()
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, body)
	}
	if !strings.Contains(body, `"code":"delete_confirmation_mismatch"`) {
		t.Fatalf("unexpected response: %s", body)
	}
}

func TestDeleteUserRequiresPasswordInRequestBody(t *testing.T) {
	setDeleteUserMockDB(t)
	recorder := httptest.NewRecorder()
	serveDeleteRequest(recorder, 1, "/users/42?confirm=target&current_password=admin-password", nil)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"reauthentication_required"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestDeleteUserRejectsSelfBeforeReauthentication(t *testing.T) {
	setDeleteUserMockDB(t)
	recorder := httptest.NewRecorder()
	serveDeleteRequest(recorder, 42, "/users/42?confirm=target", nil)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"cannot_modify_self"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestDeleteUserRejectsIncorrectPassword(t *testing.T) {
	mock := setDeleteUserMockDB(t)
	expectSuccessfulReauthentication(mock, 1, "correct-password")
	recorder := httptest.NewRecorder()
	serveDeleteRequest(recorder, 1, "/users/42?confirm=target", url.Values{
		"current_password": {"incorrect-password"},
	})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"reauthentication_required"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestDeleteReauthenticationPasswordAcceptsJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodDelete, "/users/42", strings.NewReader(`{"current_password":"secret"}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	password, err := deleteReauthenticationPassword(request)
	if err != nil {
		t.Fatal(err)
	}
	if password != "secret" {
		t.Fatalf("password = %q, want secret", password)
	}
}

func TestDeleteReauthenticationPasswordRejectsOversizedBody(t *testing.T) {
	request := httptest.NewRequest(http.MethodDelete, "/users/42", bytes.NewReader(make([]byte, maxReauthenticationBodyBytes+1)))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)

	if _, err := deleteReauthenticationPassword(request); !errors.Is(err, errInvalidReauthentication) {
		t.Fatalf("deleteReauthenticationPassword() error = %v, want errInvalidReauthentication", err)
	}
}

func serveDeleteRequest(recorder *httptest.ResponseRecorder, actorID int64, target string, form url.Values) {
	e := echo.New()
	e.DELETE("/users/:id", func(c *echo.Context) error {
		c.Set("uid", actorID)
		return del(c)
	})
	var body *strings.Reader
	if form == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(form.Encode())
	}
	request := httptest.NewRequest(http.MethodDelete, target, body)
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	e.ServeHTTP(recorder, request)
}

func expectSuccessfulReauthentication(mock sqlmock.Sqlmock, actorID int64, password string) {
	salt := "legacy-salt"
	storedHash := utils.SHA256(password + salt + config.ReadDynamicConf().Token)
	mock.ExpectQuery(`SELECT id, email, password, totp, salt, is_admin, verified FROM users WHERE id = \$1`).
		WithArgs(actorID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "email", "password", "totp", "salt", "is_admin", "verified",
		}).AddRow(actorID, "admin@example.com", storedHash, nil, salt, true, true))
}

func expectLockedAdmins(mock sqlmock.Sqlmock, ids ...int64) {
	rows := sqlmock.NewRows([]string{"id", "password"})
	for _, id := range ids {
		password := "target-hash"
		if id == 1 {
			salt := "legacy-salt"
			password = utils.SHA256("admin-password" + salt + config.ReadDynamicConf().Token)
		}
		rows.AddRow(id, password)
	}
	mock.ExpectQuery(`SELECT id, password FROM users WHERE is_admin = true ORDER BY id FOR UPDATE`).
		WillReturnRows(rows)
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
