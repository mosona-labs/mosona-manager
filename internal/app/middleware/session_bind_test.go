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
	oldBind := config.DynamicConf.SessionBindIP
	oldTrust := config.Conf.TrustProxy
	t.Cleanup(func() {
		config.DynamicConf.SessionBindIP = oldBind
		config.Conf.TrustProxy = oldTrust
	})

	e := echo.New()
	e.IPExtractor = func(req *http.Request) string {
		if config.Conf.TrustProxy {
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

	config.Conf.TrustProxy = true
	config.DynamicConf.SessionBindIP = true
	req.Header.Set("CF-Connecting-IP", "203.0.113.10")
	if !SessionBindingOK(c, sess) {
		t.Fatal("expected client IP match via RealIP")
	}

	req.Header.Set("CF-Connecting-IP", "198.51.100.2")
	c = e.NewContext(req, rec)
	if SessionBindingOK(c, sess) {
		t.Fatal("expected IP mismatch")
	}

	config.DynamicConf.SessionBindIP = false
	if !SessionBindingOK(c, sess) {
		t.Fatal("expected IP check skipped when disabled")
	}

	sess.Values["user_agent"] = "other"
	if SessionBindingOK(c, sess) {
		t.Fatal("expected UA mismatch")
	}
}
