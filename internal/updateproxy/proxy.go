package updateproxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"mosona-manager/internal/githubrelease"
)

const refreshInterval = 12 * time.Hour

type PlatformRelease struct {
	AssetName         string
	SHA256            string
	GitHubDownloadURL string
}

type cacheState struct {
	mu sync.RWMutex

	releaseTag  string
	releaseETag string
	cachedAt    time.Time
	lastError   string
	hasSuccess  bool
	byPlatform  map[string]PlatformRelease
}

var state cacheState

func Init() {
	go runRefreshLoop()
}

func runRefreshLoop() {
	refreshOnce()
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		refreshOnce()
	}
}

func refreshOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := Refresh(ctx); err != nil {
		state.mu.Lock()
		state.lastError = err.Error()
		state.mu.Unlock()
	}
}

func Refresh(ctx context.Context) error {
	client := githubrelease.NewClient()

	state.mu.RLock()
	prevETag := state.releaseETag
	state.mu.RUnlock()

	rel, etag, notModified, err := githubrelease.FetchLatest(ctx, client, prevETag)
	if err != nil {
		return err
	}
	if notModified {
		state.mu.Lock()
		if etag != "" {
			state.releaseETag = etag
		}
		state.mu.Unlock()
		return nil
	}

	platforms := []struct{ os, arch string }{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
		{"windows", "arm64"},
	}

	byPlatform := make(map[string]PlatformRelease, len(platforms))
	for _, p := range platforms {
		assetName, err := githubrelease.AssetNameForPlatform(p.os, p.arch)
		if err != nil {
			return err
		}
		checksumName, err := githubrelease.ChecksumNameForPlatform(p.os, p.arch)
		if err != nil {
			return err
		}
		checksumAsset, ok := githubrelease.FindAsset(rel.Assets, checksumName)
		if !ok {
			return fmt.Errorf("release asset %q not found", checksumName)
		}
		sum, err := downloadChecksum(ctx, client, checksumAsset.BrowserDownloadURL)
		if err != nil {
			return err
		}
		binaryAsset, ok := githubrelease.FindAsset(rel.Assets, assetName)
		if !ok {
			return fmt.Errorf("release asset %q not found", assetName)
		}
		key := platformKey(p.os, p.arch)
		byPlatform[key] = PlatformRelease{
			AssetName:         assetName,
			SHA256:            sum,
			GitHubDownloadURL: binaryAsset.BrowserDownloadURL,
		}
	}

	state.mu.Lock()
	state.releaseTag = rel.TagName
	if etag != "" {
		state.releaseETag = etag
	}
	state.cachedAt = time.Now().UTC()
	state.lastError = ""
	state.hasSuccess = true
	state.byPlatform = byPlatform
	state.mu.Unlock()
	return nil
}

func downloadChecksum(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download checksum: http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	return githubrelease.ParseChecksumFile(data)
}

func platformKey(osName, arch string) string {
	return osName + "/" + arch
}

type LatestMetadata struct {
	ReleaseTag string
	AssetName  string
	SHA256     string
	CachedAt   time.Time
}

func GetLatest(osName, arch string) (LatestMetadata, error) {
	if _, err := githubrelease.AssetNameForPlatform(osName, arch); err != nil {
		return LatestMetadata{}, err
	}
	key := platformKey(osName, arch)
	state.mu.RLock()
	defer state.mu.RUnlock()
	if !state.hasSuccess {
		return LatestMetadata{}, fmt.Errorf("release cache not ready")
	}
	pr, ok := state.byPlatform[key]
	if !ok {
		return LatestMetadata{}, fmt.Errorf("platform not in cache")
	}
	return LatestMetadata{
		ReleaseTag: state.releaseTag,
		AssetName:  pr.AssetName,
		SHA256:     pr.SHA256,
		CachedAt:   state.cachedAt,
	}, nil
}

func GetGitHubDownloadURL(osName, arch string) (string, string, error) {
	if _, err := githubrelease.AssetNameForPlatform(osName, arch); err != nil {
		return "", "", err
	}
	key := platformKey(osName, arch)
	state.mu.RLock()
	defer state.mu.RUnlock()
	if !state.hasSuccess {
		return "", "", fmt.Errorf("release cache not ready")
	}
	pr, ok := state.byPlatform[key]
	if !ok {
		return "", "", fmt.Errorf("platform not in cache")
	}
	return pr.GitHubDownloadURL, pr.AssetName, nil
}

func SetCacheForTest(tag string, cachedAt time.Time, platforms map[string]PlatformRelease) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.releaseTag = tag
	state.cachedAt = cachedAt
	state.hasSuccess = true
	state.lastError = ""
	state.byPlatform = platforms
}

func ResetForTest() {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.releaseTag = ""
	state.releaseETag = ""
	state.cachedAt = time.Time{}
	state.lastError = ""
	state.hasSuccess = false
	state.byPlatform = nil
}
