package updateproxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mosona-manager/internal/githubrelease"
)

func TestGetLatestNotReady(t *testing.T) {
	ResetForTest()
	_, err := GetLatest("linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected not ready, got %v", err)
	}
}

func TestGetLatestFromCache(t *testing.T) {
	ResetForTest()
	const checksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cachedAt := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	SetCacheForTest("v1.2.3", cachedAt, map[string]PlatformRelease{
		"linux/amd64": {
			AssetName:         "agent_linux_amd64",
			SHA256:            checksum,
			GitHubDownloadURL: "https://example.com/bin",
		},
	})
	meta, err := GetLatest("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ReleaseTag != "v1.2.3" || meta.SHA256 != checksum {
		t.Fatalf("meta: %+v", meta)
	}
}

func TestRefreshKeepsCacheOnFailure(t *testing.T) {
	ResetForTest()
	const checksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	SetCacheForTest("v0.9.0", time.Now().UTC(), map[string]PlatformRelease{
		"linux/amd64": {AssetName: "agent_linux_amd64", SHA256: checksum, GitHubDownloadURL: "https://example.com/bin"},
	})

	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	orig := githubrelease.APILatestTemplate
	githubrelease.APILatestTemplate = failSrv.URL + "/repos/%s/releases/latest"
	defer func() { githubrelease.APILatestTemplate = orig }()

	if err := Refresh(context.Background()); err == nil {
		t.Fatal("expected refresh error")
	}
	meta, err := GetLatest("linux", "amd64")
	if err != nil || meta.ReleaseTag != "v0.9.0" {
		t.Fatalf("cache should remain: %v %+v", err, meta)
	}
}

func TestRefreshSuccess(t *testing.T) {
	ResetForTest()
	const checksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/latest"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v1.2.3",
				"assets": []map[string]string{
					{"name": "agent_linux_amd64.sha256", "browser_download_url": "http://" + r.Host + "/sha"},
					{"name": "agent_linux_amd64", "browser_download_url": "http://" + r.Host + "/bin"},
					{"name": "agent_linux_arm64.sha256", "browser_download_url": "http://" + r.Host + "/sha-arm64"},
					{"name": "agent_linux_arm64", "browser_download_url": "http://" + r.Host + "/bin-arm64"},
					{"name": "agent_darwin_amd64.sha256", "browser_download_url": "http://" + r.Host + "/sha-darwin-amd64"},
					{"name": "agent_darwin_amd64", "browser_download_url": "http://" + r.Host + "/bin-darwin-amd64"},
					{"name": "agent_darwin_arm64.sha256", "browser_download_url": "http://" + r.Host + "/sha-darwin-arm64"},
					{"name": "agent_darwin_arm64", "browser_download_url": "http://" + r.Host + "/bin-darwin-arm64"},
					{"name": "agent_windows_amd64.exe.sha256", "browser_download_url": "http://" + r.Host + "/sha-win-amd64"},
					{"name": "agent_windows_amd64.exe", "browser_download_url": "http://" + r.Host + "/bin-win-amd64"},
					{"name": "agent_windows_arm64.exe.sha256", "browser_download_url": "http://" + r.Host + "/sha-win-arm64"},
					{"name": "agent_windows_arm64.exe", "browser_download_url": "http://" + r.Host + "/bin-win-arm64"},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/sha"):
			_, _ = w.Write([]byte(checksum + "  agent_linux_amd64\n"))
		case strings.Contains(r.URL.Path, "/sha-"):
			_, _ = w.Write([]byte(checksum + "  asset\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	orig := githubrelease.APILatestTemplate
	githubrelease.APILatestTemplate = srv.URL + "/repos/%s/releases/latest"
	defer func() { githubrelease.APILatestTemplate = orig }()

	if err := Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	meta, err := GetLatest("linux", "amd64")
	if err != nil || meta.ReleaseTag != "v1.2.3" {
		t.Fatalf("meta: %v %+v", err, meta)
	}
}
