package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo/v5"
	"mosona-manager/internal/config"
)

func TestSessionBindingOK(t *testing.T) {
	old := config.ReadDynamicConf()
	t.Cleanup(func() { config.ReplaceDynamicConf(old) })

	e := echo.New()
	e.IPExtractor = func(req *http.Request) string {
		if config.ReadDynamicConf().TrustProxy {
			if v := req.Header.Get("CF-Connecting-IP"); v != "" {
				return v
			}
		}
		return "10.0.0.8"
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "test-ua")
	req.RemoteAddr = "10.0.0.8:443"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	sess := &sessions.Session{Values: map[interface{}]interface{}{
		"user_agent": "test-ua",
		"client_ip":  "203.0.113.10",
	}}

	next := old
	next.TrustProxy = true
	next.SessionBindIP = true
	config.ReplaceDynamicConf(next)
	req.Header.Set("CF-Connecting-IP", "203.0.113.10")
	if !SessionBindingOK(c, sess) {
		t.Fatal("expected client IP match via RealIP")
	}

	req.Header.Set("CF-Connecting-IP", "198.51.100.2")
	c = e.NewContext(req, rec)
	if SessionBindingOK(c, sess) {
		t.Fatal("expected IP mismatch")
	}

	next.SessionBindIP = false
	config.ReplaceDynamicConf(next)
	if !SessionBindingOK(c, sess) {
		t.Fatal("expected IP check skipped when disabled")
	}

	sess.Values["user_agent"] = "other"
	if SessionBindingOK(c, sess) {
		t.Fatal("expected UA mismatch")
	}
}
