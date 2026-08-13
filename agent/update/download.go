package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	agentruntime "mosona-manager/agent/runtime"
)

type DownloadError struct {
	Err error
}

func (e *DownloadError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "download failed"
}

func (e *DownloadError) Unwrap() error {
	return e.Err
}

func IsDownloadError(err error) bool {
	var de *DownloadError
	return errors.As(err, &de)
}

func downloadHTTPClient(downloadURL string) *http.Client {
	if isHubProxyDownloadURL(downloadURL) {
		return hubHTTPClient()
	}
	return newGitHubClient()
}

func downloadToFile(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if isHubProxyDownloadURL(url) {
		if err := setHubDownloadAuthHeaders(req); err != nil {
			return &DownloadError{Err: err}
		}
	}
	req.Header.Set("User-Agent", "mosona-manager-agent/"+agentruntime.Version)
	resp, err := client.Do(req)
	if err != nil {
		return &DownloadError{Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &DownloadError{Err: fmt.Errorf("download binary: http %d", resp.StatusCode)}
	}

	tmp := dest + ".part"
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return &DownloadError{Err: copyErr}
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
