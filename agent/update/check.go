package update

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func logHubCheckFallback(err error) {
	log.Printf("auto-update: hub check failed, falling back to GitHub: %v", err)
}

type CheckResult struct {
	UpdateAvailable bool
	CurrentSHA256   string
	RemoteSHA256    string
	ReleaseTag      string
	AssetName       string
	DownloadURL     string
	Source          string // "hub" | "github"
}

func Check(ctx context.Context) (CheckResult, error) {
	if res, used, err := checkViaHub(ctx); used {
		if err != nil {
			logHubCheckFallback(err)
			return checkViaGitHub(ctx)
		}
		return res, nil
	}
	return checkViaGitHub(ctx)
}

func checkViaGitHub(ctx context.Context) (CheckResult, error) {
	var res CheckResult

	assetName, err := AssetBaseName()
	if err != nil {
		return res, err
	}
	checksumName, err := AssetChecksumName()
	if err != nil {
		return res, err
	}
	res.AssetName = assetName

	execPath, err := os.Executable()
	if err != nil {
		return res, err
	}
	res.CurrentSHA256, err = fileSHA256Hex(execPath)
	if err != nil {
		return res, err
	}

	st := loadState()
	client := newGitHubClient()
	rel, etag, notModified, err := fetchLatestRelease(ctx, client, st.ReleaseETag)
	if err != nil {
		return res, err
	}

	if notModified {
		res.RemoteSHA256 = st.LastRemoteSHA
		res.ReleaseTag = st.LastReleaseTag
		res.DownloadURL = st.LastDownloadURL
		if res.RemoteSHA256 == "" {
			st.ReleaseETag = ""
			saveState(st)
			return Check(ctx)
		}
		if strings.EqualFold(res.CurrentSHA256, res.RemoteSHA256) {
			return res, nil
		}
		if res.DownloadURL == "" {
			st.ReleaseETag = ""
			saveState(st)
			return Check(ctx)
		}
		res.UpdateAvailable = true
		res.Source = "github"
		return res, nil
	}

	if etag != "" {
		st.ReleaseETag = etag
	}

	checksumAsset, ok := findAsset(rel.Assets, checksumName)
	if !ok {
		return res, fmt.Errorf("release asset %q not found", checksumName)
	}
	remoteSum, err := downloadChecksum(ctx, client, checksumAsset.BrowserDownloadURL)
	if err != nil {
		return res, err
	}

	st.LastRemoteSHA = remoteSum
	st.LastReleaseTag = rel.TagName
	st.LastChecked = time.Now()

	res.RemoteSHA256 = remoteSum
	res.ReleaseTag = rel.TagName

	if strings.EqualFold(res.CurrentSHA256, res.RemoteSHA256) {
		st.LastDownloadURL = ""
		saveState(st)
		return res, nil
	}

	binaryAsset, ok := findAsset(rel.Assets, assetName)
	if !ok {
		return res, fmt.Errorf("release asset %q not found", assetName)
	}
	st.LastDownloadURL = binaryAsset.BrowserDownloadURL
	saveState(st)

	res.UpdateAvailable = true
	res.DownloadURL = binaryAsset.BrowserDownloadURL
	res.Source = "github"
	return res, nil
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
	return parseChecksumFile(data)
}
