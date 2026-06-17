package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mosona-manager/agent/runtime"
	"net/http"
	"strings"
	"time"
)

const (
	githubAPILatest = "https://api.github.com/repos/%s/releases/latest"
	githubAccept    = "application/vnd.github+json"
)

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type latestRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

func fetchLatestRelease(ctx context.Context, client *http.Client, ifNoneMatch string) (*latestRelease, string, bool, error) {
	url := fmt.Sprintf(githubAPILatest, runtime.ReleaseSlug())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", false, err
	}
	req.Header.Set("Accept", githubAccept)
	req.Header.Set("User-Agent", "mosona-manager-agent/"+runtime.Version)
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

	var rel latestRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&rel); err != nil {
		return nil, "", false, err
	}
	return &rel, etag, false, nil
}

func findAsset(assets []releaseAsset, name string) (releaseAsset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return releaseAsset{}, false
}

func newGitHubClient() *http.Client {
	return &http.Client{Timeout: 45 * time.Second}
}
