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

func TestPassiveAuthRejectsMissingHeaders(t *testing.T) {
	e := echo.New()
	var reached bool
	h := PassiveAuthWithLookup(func(string) (int64, string, error) {
		return 1, "", nil
	})(func(c *echo.Context) error {
		reached = true
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/agent/update/download", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h(c); err != nil {
		t.Fatal(err)
	}
	if reached {
		t.Fatal("handler should not run")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	var body _type.H
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "unauthorized" {
		t.Fatalf("code %q", body.Code)
	}
}

func TestPassiveAuthAcceptsValidSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})
	agentID := "550e8400-e29b-41d4-a716-446655440000"

	ts := time.Now().Unix()
	nonce := "YWJjZGVmZ2hpamsxMjM0NTY3ODkw"
	sig, err := identity.SignHeaders(priv, agentID, ts, nonce)
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	var reached bool
	h := passiveAuthWithLookup(func(c *echo.Context) error {
		reached = true
		if c.Get("server_id") != int64(42) {
			t.Fatalf("server_id %v", c.Get("server_id"))
		}
		return c.NoContent(http.StatusOK)
	}, func(uid string) (int64, string, error) {
		if uid != agentID {
			t.Fatalf("uid %q", uid)
		}
		return 42, string(pubPEM), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/agent/update/download?os=linux&arch=amd64", nil)
	req.Header.Set("X-Agent-Id", agentID)
	req.Header.Set("X-Agent-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Agent-Nonce", nonce)
	req.Header.Set("X-Agent-Signature", sig)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h(c); err != nil {
		t.Fatal(err)
	}
	if !reached {
		t.Fatal("handler should run")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}
