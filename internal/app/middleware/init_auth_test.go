package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mosona-manager/internal/config"

	"github.com/labstack/echo/v5"
)

func TestInitAuthUsesSnapshotAndReturnsConflict(t *testing.T) {
	config.DConfLock.Lock()
	previous := config.DynamicConf
	config.DynamicConf.Init = true
	config.DConfLock.Unlock()
	t.Cleanup(func() {
		config.DConfLock.Lock()
		config.DynamicConf = previous
		config.DConfLock.Unlock()
	})

	e := echo.New()
	e.POST("/init", func(c *echo.Context) error {
		t.Fatal("initialized request reached handler")
		return nil
	}, InitAuth)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/init", nil))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"already_initialized"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}
