package update

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"mosona-manager/agent/config"
	agentruntime "mosona-manager/agent/runtime"
	"mosona-manager/pkg/identity"
	"mosona-manager/pkg/netutil"
)

type hubLatestResponse struct {
	Code string `json:"code"`
	Data struct {
		ReleaseTag  string `json:"release_tag"`
		AssetName   string `json:"asset_name"`
		SHA256      string `json:"sha256"`
		DownloadURL string `json:"download_url"`
	} `json:"data"`
}

func hubBaseURL() string {
	if config.Current.Mode != "passive" {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(config.Current.Hub), "/")
}

func checkViaHub(ctx context.Context) (CheckResult, bool, error) {
	base := hubBaseURL()
	if base == "" {
		return CheckResult{}, false, nil
	}

	var res CheckResult
	assetName, err := AssetBaseName()
	if err != nil {
		return res, true, err
	}
	res.AssetName = assetName

	execPath, err := os.Executable()
	if err != nil {
		return res, true, err
	}
	res.CurrentSHA256, err = fileSHA256Hex(execPath)
	if err != nil {
		return res, true, err
	}

	u, err := url.Parse(base + "/api/agent/update/latest")
	if err != nil {
		return res, true, err
	}
	q := u.Query()
	q.Set("os", runtime.GOOS)
	q.Set("arch", runtime.GOARCH)
	u.RawQuery = q.Encode()

	client := hubHTTPClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return res, true, err
	}
	req.Header.Set("User-Agent", "mosona-manager-agent/"+agentruntime.Version)

	resp, err := client.Do(req)
	if err != nil {
		return res, true, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return res, true, fmt.Errorf("hub update latest: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload hubLatestResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return res, true, err
	}
	if payload.Code != "ok" {
		return res, true, fmt.Errorf("hub update latest: unexpected code %q", payload.Code)
	}
	if payload.Data.SHA256 == "" || payload.Data.DownloadURL == "" {
		return res, true, fmt.Errorf("hub update latest: incomplete payload")
	}

	downloadURL, err := resolveHubDownloadURL(base, payload.Data.DownloadURL)
	if err != nil {
		return res, true, err
	}

	res.RemoteSHA256 = strings.ToLower(payload.Data.SHA256)
	res.ReleaseTag = payload.Data.ReleaseTag
	res.DownloadURL = downloadURL
	if payload.Data.AssetName != "" {
		res.AssetName = payload.Data.AssetName
	}

	if strings.EqualFold(res.CurrentSHA256, res.RemoteSHA256) {
		res.Source = "hub"
		return res, true, nil
	}
	res.UpdateAvailable = true
	res.Source = "hub"
	return res, true, nil
}

func isHubProxyDownloadURL(raw string) bool {
	base := hubBaseURL()
	if base == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	baseU, err := url.Parse(base)
	if err != nil {
		return false
	}
	if u.Host == "" {
		return false
	}
	if !strings.EqualFold(u.Host, baseU.Host) {
		return false
	}
	return strings.HasSuffix(u.Path, "/api/agent/update/download")
}

func resolveHubDownloadURL(base, rawDownloadURL string) (string, error) {
	ref, err := url.Parse(rawDownloadURL)
	if err != nil {
		return "", err
	}
	if ref.IsAbs() {
		return ref.String(), nil
	}

	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return "", fmt.Errorf("invalid hub URL")
	}
	return baseURL.ResolveReference(ref).String(), nil
}

func setHubDownloadAuthHeaders(req *http.Request) error {
	if strings.TrimSpace(config.Current.UUID) == "" {
		return fmt.Errorf("missing agent id for hub download")
	}
	if len(config.PrivateKey) == 0 {
		if err := config.LoadPrivateKey(); err != nil {
			return fmt.Errorf("load private key: %w", err)
		}
	}
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)
	ts := time.Now().Unix()
	signature, err := identity.SignHeaders(config.PrivateKey, config.Current.UUID, ts, nonce)
	if err != nil {
		return err
	}
	req.Header.Set("X-Agent-Id", config.Current.UUID)
	req.Header.Set("X-Agent-Timestamp", fmt.Sprintf("%d", ts))
	req.Header.Set("X-Agent-Nonce", nonce)
	req.Header.Set("X-Agent-Signature", signature)
	return nil
}

func hubHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	ipPreference := config.Current.IPPreference
	if ipPreference != "" {
		network, err := netutil.NetworkForIPPreference(ipPreference)
		if err == nil {
			netDialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
			transport.DialContext = func(ctx context.Context, networkAddr, addr string) (net.Conn, error) {
				return netDialer.DialContext(ctx, network, addr)
			}
		}
	}
	return &http.Client{Timeout: 45 * time.Second, Transport: transport}
}
