package githubrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mosona-manager/agent/runtime"
)

var (
	APILatestTemplate = "https://api.github.com/repos/%s/releases/latest"
)

const (
	Accept = "application/vnd.github+json"
)

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type LatestRelease struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

func FetchLatest(ctx context.Context, client *http.Client, ifNoneMatch string) (*LatestRelease, string, bool, error) {
	url := fmt.Sprintf(APILatestTemplate, runtime.ReleaseSlug())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", false, err
	}
	req.Header.Set("Accept", Accept)
	req.Header.Set("User-Agent", "mosona-manager-hub")
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", false, err
	}
	defer func() { _ = resp.Body.Close() }()

	etag := resp.Header.Get("ETag")
	if resp.StatusCode == http.StatusNotModified {
		return nil, etag, true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", false, fmt.Errorf("no latest release found")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", false, fmt.Errorf("github api %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel LatestRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&rel); err != nil {
		return nil, "", false, err
	}
	return &rel, etag, false, nil
}

func FindAsset(assets []Asset, name string) (Asset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

func NewClient() *http.Client {
	return &http.Client{Timeout: 45 * time.Second}
}
