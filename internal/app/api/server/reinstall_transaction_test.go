package aserver

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"mosona-manager/internal/utils/encrypt"
	"mosona-manager/pkg/identity"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

func setupReinstallTest(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	oldEncryptionKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	oldStopServer := reinstallStopServer
	oldReconcileServer := reinstallReconcileServer
	reinstallStopServer = func(int64) {}
	reinstallReconcileServer = func(int64) error { return nil }
	oldDynamicConf := config.ReadDynamicConf()
	nextDynamicConf := oldDynamicConf
	nextDynamicConf.Token = "test-token-salt"
	nextDynamicConf.Domain = "https://hub.example.com"
	config.ReplaceDynamicConf(nextDynamicConf)
	t.Cleanup(func() {
		config.ReplaceDynamicConf(oldDynamicConf)
		db.Db = oldDB
		encrypt.Key = oldEncryptionKey
		reinstallStopServer = oldStopServer
		reinstallReconcileServer = oldReconcileServer
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})
	return mock
}

func serveReinstall(t *testing.T, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
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
	return rec
}

func expectLockedServerType(mock sqlmock.Sqlmock, serverType int16) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT type FROM servers WHERE id = $1 AND team_id = $2 FOR UPDATE")).
		WithArgs(int64(91), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"type"}).AddRow(serverType))
}

func TestReinstallRejectsInvalidInputBeforeTransaction(t *testing.T) {
	tests := []struct {
		name     string
		form     url.Values
		wantCode string
	}{
		{name: "missing mode", form: url.Values{}, wantCode: "invalid_mode"},
		{name: "malformed mode", form: url.Values{"mode": {"active"}}, wantCode: "invalid_mode"},
		{name: "SSH mode", form: url.Values{"mode": {"0"}}, wantCode: "invalid_mode"},
		{name: "unsupported mode", form: url.Values{"mode": {"3"}}, wantCode: "invalid_mode"},
		{name: "active address missing", form: url.Values{"mode": {"1"}, "port": {"443"}}, wantCode: "invalid_agent_address"},
		{name: "active address blank", form: url.Values{"mode": {"1"}, "address": {"  "}, "port": {"443"}}, wantCode: "invalid_agent_address"},
		{name: "active port malformed", form: url.Values{"mode": {"1"}, "address": {"agent.example.com"}, "port": {"abc"}}, wantCode: "invalid_agent_address"},
		{name: "active port zero", form: url.Values{"mode": {"1"}, "address": {"agent.example.com"}, "port": {"0"}}, wantCode: "invalid_agent_address"},
		{name: "active port too large", form: url.Values{"mode": {"1"}, "address": {"agent.example.com"}, "port": {"65536"}}, wantCode: "invalid_agent_address"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupReinstallTest(t)
			rec := serveReinstall(t, tt.form)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"`+tt.wantCode+`"`) {
				t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestReinstallRejectsMissingOrIncompatibleServer(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		serverType *int16
		wantCode   string
	}{
		{name: "missing server", mode: "1", wantCode: "error"},
		{name: "SSH server", mode: "1", serverType: int16Pointer(0), wantCode: "invalid_server_type"},
		{name: "active to passive", mode: "2", serverType: int16Pointer(1), wantCode: "mode_mismatch"},
		{name: "passive to active", mode: "1", serverType: int16Pointer(2), wantCode: "mode_mismatch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := setupReinstallTest(t)
			mock.ExpectBegin()
			query := mock.ExpectQuery(regexp.QuoteMeta("SELECT type FROM servers WHERE id = $1 AND team_id = $2 FOR UPDATE")).
				WithArgs(int64(91), int64(7))
			if tt.serverType == nil {
				query.WillReturnRows(sqlmock.NewRows([]string{"type"}))
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"type"}).AddRow(*tt.serverType))
			}
			mock.ExpectRollback()

			form := url.Values{"mode": {tt.mode}}
			if tt.mode == "1" {
				form.Set("address", "agent.example.com")
				form.Set("port", "443")
			}
			rec := serveReinstall(t, form)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"`+tt.wantCode+`"`) {
				t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestReinstallActiveReplacesAgentStateAtomically(t *testing.T) {
	mock := setupReinstallTest(t)
	auditCalls := captureReinstallAuditCalls(t)
	stopCalls, reconcileCalls := captureReinstallLifecycleCalls(t, mock)
	expectLockedServerType(mock, 1)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE servers SET type = $1, updated_at = now() WHERE id = $2 AND team_id = $3")).
		WithArgs(1, int64(91), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM enroll_tokens WHERE server_id = $1")).
		WithArgs(int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM agents WHERE server_id = $1")).
		WithArgs(int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO agents (server_id, agent_uid, status, host, port, private_key) VALUES ($1, $2, $3, $4, $5, $6)")).
		WithArgs(int64(91), sqlmock.AnyArg(), 0, "agent.example.com", 443, encryptedAgentPrivateKeyMatcher{serverID: 91}).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	rec := serveReinstall(t, url.Values{
		"mode":    {"1"},
		"address": {" agent.example.com "},
		"port":    {"443"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec.Body.Bytes())
	if data["host"] != "agent.example.com" || data["public_key"] == "" {
		t.Fatalf("unexpected response data: %#v", data)
	}
	if *auditCalls != 1 {
		t.Fatalf("audit calls = %d, want 1", *auditCalls)
	}
	if *stopCalls != 1 || *reconcileCalls != 1 {
		t.Fatalf("lifecycle calls = stop %d, reconcile %d; want 1 each", *stopCalls, *reconcileCalls)
	}
}

func TestReinstallPassiveReplacesAgentStateAtomically(t *testing.T) {
	mock := setupReinstallTest(t)
	auditCalls := captureReinstallAuditCalls(t)
	expectLockedServerType(mock, 2)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE servers SET type = $1, updated_at = now() WHERE id = $2 AND team_id = $3")).
		WithArgs(2, int64(91), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM agents WHERE server_id = $1")).
		WithArgs(int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	var insertedHash string
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO enroll_tokens (server_id, token_hash, is_revoked, created_at)
		 VALUES ($1, $2, FALSE, NOW())
		 ON CONFLICT (server_id) DO UPDATE
		 SET token_hash = EXCLUDED.token_hash,
		     is_revoked = FALSE,
		     created_at = NOW()`)).
		WithArgs(int64(91), capturedString{target: &insertedHash}).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	rec := serveReinstall(t, url.Values{"mode": {"2"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec.Body.Bytes())
	if data["hub"] != "https://hub.example.com" {
		t.Fatalf("unexpected response data: %#v", data)
	}
	token := data["enroll_token"]
	if len(token) != 32 {
		t.Fatalf("enroll token length = %d, want 32", len(token))
	}
	if insertedHash != utils.SHA256(token+"test-token-salt") {
		t.Fatal("stored token hash does not match the returned enrollment token")
	}
	if *auditCalls != 1 {
		t.Fatalf("audit calls = %d, want 1", *auditCalls)
	}
}

func TestReinstallActiveInsertFailureRollsBackWithoutStoppingConnections(t *testing.T) {
	mock := setupReinstallTest(t)
	stopCalls, reconcileCalls := captureReinstallLifecycleCalls(t, mock)
	expectLockedServerType(mock, 1)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE servers SET type = $1, updated_at = now() WHERE id = $2 AND team_id = $3")).
		WithArgs(1, int64(91), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM enroll_tokens WHERE server_id = $1")).
		WithArgs(int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM agents WHERE server_id = $1")).
		WithArgs(int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO agents (server_id, agent_uid, status, host, port, private_key) VALUES ($1, $2, $3, $4, $5, $6)")).
		WithArgs(int64(91), sqlmock.AnyArg(), 0, "agent.example.com", 443, encryptedAgentPrivateKeyMatcher{serverID: 91}).
		WillReturnError(errors.New("duplicate key value"))
	mock.ExpectRollback()

	rec := serveReinstall(t, url.Values{
		"mode":    {"1"},
		"address": {"agent.example.com"},
		"port":    {"443"},
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	if *stopCalls != 0 || *reconcileCalls != 0 {
		t.Fatalf("lifecycle calls after failed insert = stop %d, reconcile %d", *stopCalls, *reconcileCalls)
	}
}

func TestReinstallCommitFailureDoesNotStopConnections(t *testing.T) {
	mock := setupReinstallTest(t)
	stopCalls, reconcileCalls := captureReinstallLifecycleCalls(t, mock)
	expectLockedServerType(mock, 2)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE servers SET type = $1, updated_at = now() WHERE id = $2 AND team_id = $3")).
		WithArgs(2, int64(91), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM agents WHERE server_id = $1")).
		WithArgs(int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO enroll_tokens").
		WithArgs(int64(91), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	rec := serveReinstall(t, url.Values{"mode": {"2"}})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	if *stopCalls != 0 || *reconcileCalls != 0 {
		t.Fatalf("lifecycle calls after failed commit = stop %d, reconcile %d", *stopCalls, *reconcileCalls)
	}
}

func int16Pointer(value int16) *int16 {
	return &value
}

func captureReinstallAuditCalls(t *testing.T) *int {
	t.Helper()
	calls := 0
	oldLogAdd := reinstallLogAdd
	reinstallLogAdd = func(teamID, userID int64, category, message, ip, ua, level string) {
		calls++
	}
	t.Cleanup(func() {
		reinstallLogAdd = oldLogAdd
	})
	return &calls
}

func captureReinstallLifecycleCalls(t *testing.T, mock sqlmock.Sqlmock) (*int, *int) {
	t.Helper()
	stopCalls := 0
	reconcileCalls := 0
	oldStopServer := reinstallStopServer
	oldReconcileServer := reinstallReconcileServer
	reinstallStopServer = func(int64) {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("connection stopped before transaction completed: %v", err)
		}
		stopCalls++
	}
	reinstallReconcileServer = func(int64) error {
		reconcileCalls++
		return nil
	}
	t.Cleanup(func() {
		reinstallStopServer = oldStopServer
		reinstallReconcileServer = oldReconcileServer
	})
	return &stopCalls, &reconcileCalls
}

func responseData(t *testing.T, body []byte) map[string]string {
	t.Helper()
	var response struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := make(map[string]string, len(response.Data))
	for key, value := range response.Data {
		if stringValue, ok := value.(string); ok {
			data[key] = stringValue
		}
	}
	return data
}

type capturedString struct {
	target *string
}

type encryptedAgentPrivateKeyMatcher struct {
	serverID int64
}

func (matcher encryptedAgentPrivateKeyMatcher) Match(value driver.Value) bool {
	ciphertext, ok := value.([]byte)
	if !ok || !encrypt.IsCurrentCiphertext(ciphertext) {
		return false
	}
	plaintext, err := encrypt.Decrypt(ciphertext, encrypt.Key, encrypt.AgentPrivateKeyContext(matcher.serverID))
	if err != nil {
		return false
	}
	_, err = identity.ParseEd25519PrivateKeyPEM(plaintext)
	return err == nil
}

func (capture capturedString) Match(value driver.Value) bool {
	stringValue, ok := value.(string)
	if ok {
		*capture.target = stringValue
	}
	return ok
}
