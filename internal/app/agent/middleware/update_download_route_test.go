package middleware

import (
	"crypto/ed25519"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"mosona-manager/internal/_type"
	"mosona-manager/pkg/identity"

	"github.com/labstack/echo/v5"
)

func TestUpdateDownloadRouteRejectsWithoutAuth(t *testing.T) {
	e := echo.New()
	g := e.Group("/api/agent")
	g.GET("/update/download", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	}, PassiveAuthWithLookup(func(string) (int64, string, error) {
		return 1, "", nil
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/agent/update/download?os=linux&arch=amd64", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body _type.H
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "unauthorized" {
		t.Fatalf("code %q", body.Code)
	}
}

func TestUpdateDownloadRouteAllowsValidAuth(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})
	agentID := "550e8400-e29b-41d4-a716-446655440002"

	var handlerHit bool
	e := echo.New()
	g := e.Group("/api/agent")
	g.GET("/update/download", func(c *echo.Context) error {
		handlerHit = true
		return c.NoContent(http.StatusOK)
	}, func(next echo.HandlerFunc) echo.HandlerFunc {
		return passiveAuthWithLookup(next, func(uid string) (int64, string, error) {
			return 7, string(pubPEM), nil
		})
	})

	ts := time.Now().Unix()
	nonce := "YWJjZGVmZ2hpamsxMjM0NTY3ODkw"
	sig, err := identity.SignHeaders(priv, agentID, ts, nonce)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/agent/update/download?os=linux&arch=amd64", nil)
	req.Header.Set("X-Agent-Id", agentID)
	req.Header.Set("X-Agent-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Agent-Nonce", nonce)
	req.Header.Set("X-Agent-Signature", sig)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if !handlerHit {
		t.Fatal("download handler should run after auth")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}
