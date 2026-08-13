package muser

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v5"
)

func TestEditUserRejectsOversizedBody(t *testing.T) {
	setDeleteUserMockDB(t)
	e := echo.New()
	e.PUT("/users/:id", func(c *echo.Context) error {
		c.Set("uid", int64(1))
		return edit(c)
	})
	request := httptest.NewRequest(http.MethodPut, "/users/42", bytes.NewReader(make([]byte, maxReauthenticationBodyBytes+1)))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"reauthentication_required"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestEditUserRejectsSelfDemotionBeforeDatabaseAccess(t *testing.T) {
	setDeleteUserMockDB(t)
	recorder := httptest.NewRecorder()
	serveEditRequest(recorder, 42, "/users/42", url.Values{
		"username": {"admin"},
		"email":    {"admin@example.com"},
		"is_admin": {"false"},
	})

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"cannot_modify_self"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestEditUserDemotionRequiresReauthentication(t *testing.T) {
	mock := setDeleteUserMockDB(t)
	mock.ExpectBegin()
	expectLockedAdmins(mock, 1, 42)
	mock.ExpectQuery(`SELECT is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"is_admin"}).AddRow(true))
	mock.ExpectRollback()

	recorder := httptest.NewRecorder()
	serveEditRequest(recorder, 1, "/users/42", url.Values{
		"username": {"target"},
		"email":    {"target@example.com"},
		"is_admin": {"false"},
	})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"reauthentication_required"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestEditUserRejectsIncorrectReauthentication(t *testing.T) {
	mock := setDeleteUserMockDB(t)
	expectSuccessfulReauthentication(mock, 1, "correct-password")
	recorder := httptest.NewRecorder()
	serveEditRequest(recorder, 1, "/users/42", url.Values{
		"username":         {"target"},
		"email":            {"target@example.com"},
		"is_admin":         {"false"},
		"current_password": {"incorrect-password"},
	})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func serveEditRequest(recorder *httptest.ResponseRecorder, actorID int64, target string, form url.Values) {
	e := echo.New()
	e.PUT("/users/:id", func(c *echo.Context) error {
		c.Set("uid", actorID)
		return edit(c)
	})
	request := httptest.NewRequest(http.MethodPut, target, strings.NewReader(form.Encode()))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	e.ServeHTTP(recorder, request)
}
