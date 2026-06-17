package update

import (
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mosona-manager/agent/config"
	"mosona-manager/pkg/identity"
)

func TestSetHubDownloadAuthHeadersVerifiesWithPublicKey(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	config.PrivateKey = priv
	agentID := "550e8400-e29b-41d4-a716-446655440000"
	config.Current = config.Config{Mode: "passive", UUID: agentID, Hub: "https://hub.example.com"}

	req := httptest.NewRequest(http.MethodGet, "https://hub.example.com/api/agent/update/download?os=linux&arch=amd64", nil)
	if err := setHubDownloadAuthHeaders(req); err != nil {
		t.Fatal(err)
	}
	uid := req.Header.Get("X-Agent-Id")
	ts := req.Header.Get("X-Agent-Timestamp")
	nonce := req.Header.Get("X-Agent-Nonce")
	sig := req.Header.Get("X-Agent-Signature")
	if uid != agentID || ts == "" || len(nonce) < 16 || sig == "" {
		t.Fatalf("headers incomplete: id=%q ts=%q nonce=%q sig=%q", uid, ts, nonce, sig)
	}
	if err := identity.VerifySignedHeaders(pub, uid, ts, nonce, sig, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestSetHubDownloadAuthHeadersRequiresAgentID(t *testing.T) {
	config.Current = config.Config{Mode: "passive", Hub: "https://hub.example.com"}
	req := httptest.NewRequest(http.MethodGet, "https://hub.example.com/api/agent/update/download", nil)
	err := setHubDownloadAuthHeaders(req)
	if err == nil {
		t.Fatal("expected error without agent id")
	}
}
