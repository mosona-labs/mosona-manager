package anotification

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

func TestBeginNotificationTestLimitsConcurrentAndFrequentCalls(t *testing.T) {
	teamID := time.Now().UnixNano()
	now := time.Now()
	release, ok := beginNotificationTest(teamID, now)
	if !ok {
		t.Fatal("first test was rejected")
	}
	if _, ok := beginNotificationTest(teamID, now); ok {
		t.Fatal("concurrent test was accepted")
	}
	release()
	if _, ok := beginNotificationTest(teamID, now.Add(notificationTestInterval-time.Millisecond)); ok {
		t.Fatal("frequent test was accepted")
	}
	release, ok = beginNotificationTest(teamID, now.Add(notificationTestInterval))
	if !ok {
		t.Fatal("test was not accepted after interval")
	}
	release()
}

func TestNotificationTestRejectsPrivateGenericWebhook(t *testing.T) {
	e := echo.New()
	e.POST("/test", func(c *echo.Context) error {
		c.Set("tid", time.Now().UnixNano())
		return test(c)
	})
	form := url.Values{"uri": {"generic://127.0.0.1/hook"}}
	request := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid_notification"`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestNotificationUpdateRejectsPrivateGenericWebhook(t *testing.T) {
	e := echo.New()
	e.PUT("/notification", func(c *echo.Context) error {
		c.Set("tid", int64(91))
		return update(c)
	})
	body := []byte(`[{"module":"shoutrrr","target":"generic://127.0.0.1/hook"}]`)
	request := httptest.NewRequest(http.MethodPut, "/notification", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid_notification"`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
