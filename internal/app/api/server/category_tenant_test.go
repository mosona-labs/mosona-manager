package aserver

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

func TestEditRejectsCategoryOutsideTeam(t *testing.T) {
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

	mock.ExpectQuery(`SELECT type, allow_monitor FROM servers WHERE id=\$1 AND team_id=\$2`).
		WithArgs(int64(91), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"type", "allow_monitor"}).AddRow(1, false))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM categories WHERE team = \$1 AND id = \$2 FOR KEY SHARE`).
		WithArgs(int64(7), int64(22)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	e := echo.New()
	e.PUT("/server/:id", func(c *echo.Context) error {
		c.Set("tid", int64(7))
		c.Set("uid", int64(10))
		return edit(c)
	})
	request := httptest.NewRequest(
		http.MethodPut,
		"/server/91",
		strings.NewReader(`{"name":"server","category":22,"allow_monitor":false}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"invalid_category"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}
