package moauth

import (
	"mosona-manager/internal/db"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

const oauthListSQL = `SELECT id, name, icon, protocol, issuer_url, auth_url, token_url, userinfo_url, scopes, subject_field, identity_namespace_version, config_revision, client_id, client_secret, skip_2fa, is_enabled, sort, created_at, updated_at FROM auth_provider ORDER BY sort, id DESC LIMIT $1 OFFSET $2`

func useOAuthListMockDB(t *testing.T) sqlmock.Sqlmock {
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

func serveOAuthList(t *testing.T, target string) *httptest.ResponseRecorder {
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

func TestListRejectsInvalidPagination(t *testing.T) {
	recorder := serveOAuthList(t, "/?page=0&size=20")
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"msg":"Invalid pagination"`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestListUsesDefaultPagination(t *testing.T) {
	mock := useOAuthListMockDB(t)
	columns := []string{
		"id", "name", "icon", "protocol", "issuer_url", "auth_url", "token_url", "userinfo_url", "scopes",
		"subject_field", "identity_namespace_version", "config_revision", "client_id", "client_secret", "skip_2fa",
		"is_enabled", "sort", "created_at", "updated_at",
	}
	mock.ExpectQuery(oauthListSQL).WithArgs(20, 0).WillReturnRows(sqlmock.NewRows(columns))
	mock.ExpectQuery(`SELECT COUNT(id) FROM auth_provider`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	recorder := serveOAuthList(t, "/")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"items":[]`) ||
		!strings.Contains(recorder.Body.String(), `"total":0`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
