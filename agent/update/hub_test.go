package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mosona-manager/agent/config"
)

func TestCheckViaHubSuccess(t *testing.T) {
	const remote = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/update/latest" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("os") == "" || r.URL.Query().Get("arch") == "" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "ok",
			"data": map[string]string{
				"release_tag":  "v9.9.9",
				"asset_name":   "agent_linux_amd64",
				"sha256":       remote,
				"download_url": "/api/agent/update/download?os=linux&arch=amd64",
			},
		})
	}))
	defer srv.Close()

	prev := config.Current
	config.Current = config.Config{Mode: "passive", Hub: srv.URL}
	defer func() { config.Current = prev }()

	res, used, err := checkViaHub(context.Background())
	if err != nil || !used {
		t.Fatalf("used=%v err=%v", used, err)
	}
	wantDownloadURL := srv.URL + "/api/agent/update/download?os=linux&arch=amd64"
	if res.ReleaseTag != "v9.9.9" || res.RemoteSHA256 != remote || res.DownloadURL != wantDownloadURL || res.Source != "hub" {
		t.Fatalf("res: %+v", res)
	}
}

func TestResolveHubDownloadURLKeepsAbsoluteURL(t *testing.T) {
	got, err := resolveHubDownloadURL("https://hub.example.com", "https://cdn.example.com/agent")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://cdn.example.com/agent" {
		t.Fatalf("got %q", got)
	}
}

func TestCheckViaHubInactiveForActiveMode(t *testing.T) {
	prev := config.Current
	config.Current = config.Config{Mode: "active"}
	defer func() { config.Current = prev }()

	_, used, err := checkViaHub(context.Background())
	if err != nil || used {
		t.Fatalf("used=%v err=%v", used, err)
	}
}
