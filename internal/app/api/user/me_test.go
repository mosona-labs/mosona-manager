package auser

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	gorillasessions "github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	contribsession "github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
	"mosona-manager/internal/db"
)

func TestMeClearsStaleTeamThatStillExists(t *testing.T) {
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

	now := time.Now()
	mock.ExpectQuery(`SELECT id, username, email, is_admin.*FROM users WHERE id = \$1`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "email", "is_admin", "totp_enabled", "verified", "created_at", "pwd_at",
		}).AddRow(42, "user", "user@example.com", false, false, true, now, nil))
	mock.ExpectQuery(`SELECT role FROM m_team_user WHERE user_id = \$1 AND team_id = \$2`).
		WithArgs(int64(42), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}))
	mock.ExpectExec(`DELETE FROM users_config WHERE uid = \$1 AND active_team = \$2`).
		WithArgs(int64(42), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT t.id.*FROM teams t.*JOIN m_team_user mtu.*WHERE mtu.user_id = \$1`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "owner_id", "name", "description", "color", "image", "updated_at", "created_at",
		}))

	e := echo.New()
	e.Use(contribsession.Middleware(gorillasessions.NewCookieStore([]byte("01234567890123456789012345678901"))))
	e.GET("/me", func(c *echo.Context) error {
		c.Set("uid", int64(42))
		c.Set("tid", int64(7))
		return me(c)
	})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"team":null`) {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}
