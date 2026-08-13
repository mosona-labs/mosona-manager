package aserver

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
	"mosona-manager/internal/db"
)

func TestReinstallSSHModeRollsBackTransaction(t *testing.T) {
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

	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM servers WHERE id = \$1 AND team_id = \$2\)`).
		WithArgs(int64(91), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectRollback()

	form := url.Values{"mode": {"0"}}
	e := echo.New()
	e.POST("/server/:id/reinstall", func(c *echo.Context) error {
		c.Set("tid", int64(7))
		c.Set("uid", int64(10))
		return reinstall(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/server/91/reinstall", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "SSH Mode don't need to reinstall") {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}
