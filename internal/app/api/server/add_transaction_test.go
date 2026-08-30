package aserver

import (
	"bytes"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"mosona-manager/internal/db"
	"mosona-manager/internal/utils/encrypt"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

func setupAddTest(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	oldEncryptionKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	oldNewAgentUUID := addNewAgentUUID
	oldRemoveServerStatus := addRemoveServerStatus
	oldReconcileServer := addReconcileServer
	oldLogAdd := addLogAdd
	addNewAgentUUID = uuid.NewRandom
	addRemoveServerStatus = func(int64) error { return nil }
	addReconcileServer = func(int64) error { return nil }
	addLogAdd = func(int64, int64, string, string, string, string, string) {}
	t.Cleanup(func() {
		db.Db = oldDB
		encrypt.Key = oldEncryptionKey
		addNewAgentUUID = oldNewAgentUUID
		addRemoveServerStatus = oldRemoveServerStatus
		addReconcileServer = oldReconcileServer
		addLogAdd = oldLogAdd
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})
	return mock
}

func serveAdd(t *testing.T, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.POST("/server", func(c *echo.Context) error {
		c.Set("tid", int64(7))
		c.Set("uid", int64(10))
		return add(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/server", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, req)
	return recorder
}

func TestAddActiveEncryptsPrivateKeyAndUsesRandomUUID(t *testing.T) {
	mock := setupAddTest(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, sort FROM categories WHERE team = $1 AND id = $2")).
		WithArgs(int64(7), int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "sort"}).AddRow(int64(3), "default", 0))
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO servers (team_id, name, type, category, allow_monitor, allow_terminal, weight) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id")).
		WithArgs(int64(7), "active", 1, int64(3), true, true, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(91)))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO server_info (sid, note, provider, cycle, start_time, end_time, amount, auto_renew, bandwidth, traffic, traffic_type, note_public) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)")).
		WithArgs(int64(91), "", "", 0, sqlmock.AnyArg(), sqlmock.AnyArg(), "", false, "", "", 0, "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO server_info_adv (sid) VALUES ($1)")).
		WithArgs(int64(91)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO agents (server_id, agent_uid, status, host, port, private_key) VALUES ($1, $2, $3, $4, $5, $6)")).
		WithArgs(int64(91), randomUUIDv4Matcher{}, 0, "agent.example.com", 443, encryptedAgentPrivateKeyMatcher{serverID: 91}).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, item, threshold, for_duration\s+FROM team_alerts\s+WHERE team_id = \$1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "item", "threshold", "for_duration"}))
	mock.ExpectCommit()

	recorder := serveAdd(t, url.Values{
		"name":           {"active"},
		"mode":           {"1"},
		"category_id":    {"3"},
		"allow_monitor":  {"true"},
		"allow_terminal": {"true"},
		"address":        {"agent.example.com"},
		"port":           {"443"},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"public_key":"`) || !strings.Contains(recorder.Body.String(), `"agent_uid":"`) {
		t.Fatalf("response is missing Agent installation data: %s", recorder.Body.String())
	}
}

func TestAddActiveRollsBackWhenRandomUUIDGenerationFails(t *testing.T) {
	mock := setupAddTest(t)
	addNewAgentUUID = func() (uuid.UUID, error) {
		return uuid.Nil, errors.New("random source unavailable")
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, sort FROM categories WHERE team = $1 AND id = $2")).
		WithArgs(int64(7), int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "sort"}).AddRow(int64(3), "default", 0))
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO servers (team_id, name, type, category, allow_monitor, allow_terminal, weight) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id")).
		WithArgs(int64(7), "active", 1, int64(3), false, false, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(91)))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO server_info (sid, note, provider, cycle, start_time, end_time, amount, auto_renew, bandwidth, traffic, traffic_type, note_public) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)")).
		WithArgs(int64(91), "", "", 0, sqlmock.AnyArg(), sqlmock.AnyArg(), "", false, "", "", 0, "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO server_info_adv (sid) VALUES ($1)")).
		WithArgs(int64(91)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	recorder := serveAdd(t, url.Values{
		"name":        {"active"},
		"mode":        {"1"},
		"category_id": {"3"},
		"address":     {"agent.example.com"},
		"port":        {"443"},
	})
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "Agent ID generation error") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

type randomUUIDv4Matcher struct{}

func (randomUUIDv4Matcher) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	id, err := uuid.Parse(raw)
	return err == nil && id.Version() == 4
}
