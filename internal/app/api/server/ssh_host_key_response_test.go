package aserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestSSHHostKeyConfirmationRequiredResponse(t *testing.T) {
	e := echo.New()
	e.GET("/confirm", func(c *echo.Context) error {
		return sshHostKeyConfirmationRequired(c, sshValidationResult{
			HostKey:     "ssh-ed25519 AAAATEST",
			Fingerprint: "SHA256:test-fingerprint",
			Changed:     true,
		})
	})
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/confirm", nil))

	body := recorder.Body.String()
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, body)
	}
	for _, fragment := range []string{
		`"code":"ssh_host_key_confirmation_required"`,
		`"fingerprint":"SHA256:test-fingerprint"`,
		`"host_key":"ssh-ed25519 AAAATEST"`,
		`"changed":true`,
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("response does not contain %q: %s", fragment, body)
		}
	}
}
