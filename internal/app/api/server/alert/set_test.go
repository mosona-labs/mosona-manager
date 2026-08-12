package aalert

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

func TestSetReturnsNotFoundWhenServerIsOutsideTeam(t *testing.T) {
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
	mock.ExpectExec(`(?s)INSERT INTO server_alerts.*SELECT s.id.*WHERE s.id = \$1 AND s.team_id = \$5.*ON CONFLICT`).
		WithArgs(int64(91), "cpu_usage", 80, 10, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	form := url.Values{
		"item":         {"cpu_usage"},
		"threshold":    {"80"},
		"for_duration": {"10"},
	}
	e := echo.New()
	e.PUT("/alert/:id", func(c *echo.Context) error {
		c.Set("tid", int64(7))
		return set(c)
	})
	req := httptest.NewRequest(http.MethodPut, "/alert/91", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"code":"server_not_found"`) {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}
