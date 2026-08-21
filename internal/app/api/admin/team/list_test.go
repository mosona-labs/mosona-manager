package mteam

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
	teamListSQL          = "SELECT teams.id, teams.name, teams.description, teams.created_at, teams.updated_at FROM teams ORDER BY teams.id DESC LIMIT 20 OFFSET 0"
	teamCountSQL         = "SELECT COUNT(teams.id) FROM teams"
	teamListWithEmailSQL = `SELECT teams.id, teams.name, teams.description, teams.created_at, teams.updated_at FROM teams WHERE EXISTS (
			SELECT 1
			FROM m_team_user mtu
			JOIN users u ON mtu.user_id = u.id
			WHERE mtu.team_id = teams.id AND u.email LIKE $1 ESCAPE '\'
		) ORDER BY teams.id DESC LIMIT 20 OFFSET 0`
	teamCountWithEmailSQL = `SELECT COUNT(teams.id) FROM teams WHERE EXISTS (
			SELECT 1
			FROM m_team_user mtu
			JOIN users u ON mtu.user_id = u.id
			WHERE mtu.team_id = teams.id AND u.email LIKE $1 ESCAPE '\'
		)`
	teamListWithEmailAndSearchSQL = `SELECT teams.id, teams.name, teams.description, teams.created_at, teams.updated_at FROM teams WHERE EXISTS (
			SELECT 1
			FROM m_team_user mtu
			JOIN users u ON mtu.user_id = u.id
			WHERE mtu.team_id = teams.id AND u.email LIKE $1 ESCAPE '\'
		) AND (teams.name LIKE $2 ESCAPE '\' OR teams.description LIKE $3 ESCAPE '\') ORDER BY teams.id DESC LIMIT 10 OFFSET 10`
	teamCountWithEmailAndSearchSQL = `SELECT COUNT(teams.id) FROM teams WHERE EXISTS (
			SELECT 1
			FROM m_team_user mtu
			JOIN users u ON mtu.user_id = u.id
			WHERE mtu.team_id = teams.id AND u.email LIKE $1 ESCAPE '\'
		) AND (teams.name LIKE $2 ESCAPE '\' OR teams.description LIKE $3 ESCAPE '\')`
	teamNumericSearchListSQL  = `SELECT teams.id, teams.name, teams.description, teams.created_at, teams.updated_at FROM teams WHERE (teams.id = $1 OR teams.name LIKE $2 ESCAPE '\' OR teams.description LIKE $3 ESCAPE '\') ORDER BY teams.id DESC LIMIT 20 OFFSET 0`
	teamNumericSearchCountSQL = `SELECT COUNT(teams.id) FROM teams WHERE (teams.id = $1 OR teams.name LIKE $2 ESCAPE '\' OR teams.description LIKE $3 ESCAPE '\')`
)

func useTeamListMockDB(t *testing.T) sqlmock.Sqlmock {
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

func teamRows() *sqlmock.Rows {
	createdAt := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{"id", "name", "description", "created_at", "updated_at"}).
		AddRow(int64(7), "Production", "Primary team", createdAt, createdAt)
}

func serveTeamList(t *testing.T, target string) *httptest.ResponseRecorder {
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

func TestListWithoutEmailDoesNotJoinMembers(t *testing.T) {
	mock := useTeamListMockDB(t)
	mock.ExpectQuery(teamListSQL).WillReturnRows(teamRows())
	mock.ExpectQuery(teamCountSQL).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	recorder := serveTeamList(t, "/")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"total":1`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestListEmailAndSearchPreservePlaceholderOrderAndPagination(t *testing.T) {
	mock := useTeamListMockDB(t)
	emailPattern := `%member\%@example.com%`
	searchPattern := `%prod\_\%%`
	mock.ExpectQuery(teamListWithEmailAndSearchSQL).
		WithArgs(emailPattern, searchPattern, searchPattern).
		WillReturnRows(teamRows())
	mock.ExpectQuery(teamCountWithEmailAndSearchSQL).
		WithArgs(emailPattern, searchPattern, searchPattern).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	recorder := serveTeamList(t, "/?page=2&size=10&email=member%25%40example.com&search=prod_%25")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"total":1`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestListRejectsInvalidPagination(t *testing.T) {
	for _, target := range []string{
		"/?page=0&size=20",
		"/?page=invalid&size=20",
		"/?page=1&size=0",
		"/?page=1&size=1001",
	} {
		recorder := serveTeamList(t, target)
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"msg":"Invalid pagination"`) {
			t.Fatalf("target = %s, status = %d, body = %s", target, recorder.Code, recorder.Body.String())
		}
	}
}

func TestListNumericSearchIncludesIDCondition(t *testing.T) {
	mock := useTeamListMockDB(t)
	pattern := "%123%"
	mock.ExpectQuery(teamNumericSearchListSQL).
		WithArgs(int64(123), pattern, pattern).
		WillReturnRows(teamRows())
	mock.ExpectQuery(teamNumericSearchCountSQL).
		WithArgs(int64(123), pattern, pattern).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	recorder := serveTeamList(t, "/?search=123")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"total":1`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestListEmailFilterUsesCorrelatedExists(t *testing.T) {
	mock := useTeamListMockDB(t)
	pattern := "%member@example.com%"
	mock.ExpectQuery(teamListWithEmailSQL).WithArgs(pattern).WillReturnRows(teamRows())
	mock.ExpectQuery(teamCountWithEmailSQL).WithArgs(pattern).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	recorder := serveTeamList(t, "/?page=1&size=20&email=member%40example.com")
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"total":1`) || strings.Count(body, `"id":7`) != 1 {
		t.Fatalf("status = %d, body = %s", recorder.Code, body)
	}
}
