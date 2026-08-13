package msettings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"mosona-manager/internal/config"
	"mosona-manager/internal/db"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

func TestGetMasksConfiguredSecrets(t *testing.T) {
	previous := config.ReadDynamicConf()
	t.Cleanup(func() { config.ReplaceDynamicConf(previous) })
	config.ReplaceDynamicConf(config.DynamicConfigType{
		SMTPPassword:  "smtp-plaintext",
		CaptchaSecret: "captcha-plaintext",
	})

	e := echo.New()
	e.GET("/settings", get)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/settings", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data Response `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.SMTPPassword != secretMask {
		t.Fatalf("SMTP password = %q, want mask", response.Data.SMTPPassword)
	}
	if response.Data.CaptchaSecretKey != secretMask {
		t.Fatalf("captcha secret = %q, want mask", response.Data.CaptchaSecretKey)
	}
	for _, plaintext := range []string{"smtp-plaintext", "captcha-plaintext"} {
		if strings.Contains(recorder.Body.String(), plaintext) {
			t.Fatalf("response contains plaintext secret %q", plaintext)
		}
	}
	if maskSecret("") != "" {
		t.Fatal("empty secret must remain empty")
	}
}

func TestSetPreservesMaskedSecretAndRedactsAuditLog(t *testing.T) {
	mock := setSettingsMockDB(t)
	previous := config.ReadDynamicConf()
	t.Cleanup(func() { config.ReplaceDynamicConf(previous) })
	config.ReplaceDynamicConf(config.DynamicConfigType{Token: "existing-token"})

	mock.ExpectBegin()
	upsert := regexp.QuoteMeta(`INSERT INTO config (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2`)
	mock.ExpectExec(upsert).
		WithArgs("captcha_secret", "new-captcha-secret").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(upsert).
		WithArgs("title", "Control Plane").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT key, value FROM config")).
		WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("token", "existing-token").
			AddRow("smtp_password", "stored-smtp-secret").
			AddRow("captcha_secret", "new-captcha-secret").
			AddRow("title", "Control Plane"))

	var auditMessage string
	previousAuditLog := addSettingsAuditLog
	addSettingsAuditLog = func(_ int64, _ int64, _ string, message string, _ string, _ string, _ string) {
		auditMessage = message
	}
	t.Cleanup(func() { addSettingsAuditLog = previousAuditLog })

	body := `[
		{"key":"smtp_password","value":"********"},
		{"key":"captcha_secret","value":"new-captcha-secret"},
		{"key":"token","value":"attacker-controlled"},
		{"key":"title","value":"  Control Plane  "}
	]`
	e := echo.New()
	e.POST("/settings", set, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("uid", int64(42))
			return next(c)
		}
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if auditMessage != "updated: captcha_secret updated, title=Control Plane" {
		t.Fatalf("audit message = %q", auditMessage)
	}
	for _, forbidden := range []string{"new-captcha-secret", "stored-smtp-secret", "attacker-controlled"} {
		if strings.Contains(auditMessage, forbidden) {
			t.Fatalf("audit message contains %q: %s", forbidden, auditMessage)
		}
	}
}

func setSettingsMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	previous := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		db.Db = previous
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})
	return mock
}
