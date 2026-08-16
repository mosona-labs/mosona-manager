package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	contribsession "github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
	"mosona-manager/internal/db"
)

func TestTeamAccessRevokedClearsStaleTeamAndRejectsRequest(t *testing.T) {
	mock, cleanup := setTeamAccessMockDB(t)
	defer cleanup()
	mock.ExpectQuery(`SELECT role FROM m_team_user WHERE user_id = \$1 AND team_id = \$2`).
		WithArgs(int64(42), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}))
	mock.ExpectExec(`DELETE FROM users_config WHERE uid = \$1 AND active_team = \$2`).
		WithArgs(int64(42), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	e := newTeamAccessTestEcho()
	reached := false
	e.GET("/", func(c *echo.Context) error {
		reached = true
		return c.NoContent(http.StatusNoContent)
	}, TeamAccess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if reached {
		t.Fatal("protected handler was reached after membership revocation")
	}
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"team_access_revoked"`) {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestTeamAccessWithoutActiveTeamReturnsTeamRequired(t *testing.T) {
	mock, cleanup := setTeamAccessMockDB(t)
	defer cleanup()

	e := newTeamAccessTestEchoWithTeam(0)
	reached := false
	e.GET("/", func(c *echo.Context) error {
		reached = true
		return c.NoContent(http.StatusNoContent)
	}, TeamAccess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if reached {
		t.Fatal("protected handler was reached without an active team")
	}
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"team_required"`) {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database access: %v", err)
	}
}

func TestTeamAccessDatabaseFailureDoesNotDowngradeRole(t *testing.T) {
	mock, cleanup := setTeamAccessMockDB(t)
	defer cleanup()
	mock.ExpectQuery(`SELECT role FROM m_team_user WHERE user_id = \$1 AND team_id = \$2`).
		WithArgs(int64(42), int64(7)).
		WillReturnError(errors.New("database unavailable"))

	e := newTeamAccessTestEcho()
	e.GET("/", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, TeamAccess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
}

func TestWriteAndTerminalAuthFailClosedWithoutRole(t *testing.T) {
	for name, middleware := range map[string]echo.MiddlewareFunc{
		"write": WriteAuth, "terminal": TerminalAuth,
	} {
		t.Run(name, func(t *testing.T) {
			e := echo.New()
			e.GET("/", func(c *echo.Context) error { return c.NoContent(http.StatusNoContent) }, middleware)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
		})
	}
}

func newTeamAccessTestEcho() *echo.Echo {
	return newTeamAccessTestEchoWithTeam(7)
}

func newTeamAccessTestEchoWithTeam(teamID int64) *echo.Echo {
	e := echo.New()
	e.Use(contribsession.Middleware(sessions.NewCookieStore([]byte("01234567890123456789012345678901"))))
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("uid", int64(42))
			c.Set("tid", teamID)
			return next(c)
		}
	})
	return e
}

func setTeamAccessMockDB(t *testing.T) (sqlmock.Sqlmock, func()) {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	return mock, func() {
		db.Db = oldDB
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	}
}
