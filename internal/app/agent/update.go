package agent

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"mosona-manager/internal/_type"
	"mosona-manager/internal/githubrelease"
	"mosona-manager/internal/updateproxy"

	"github.com/labstack/echo/v5"
)

type updateLatestData struct {
	ReleaseTag  string `json:"release_tag"`
	AssetName   string `json:"asset_name"`
	SHA256      string `json:"sha256"`
	DownloadURL string `json:"download_url"`
	CachedAt    string `json:"cached_at"`
}

func updateLatest(c *echo.Context) error {
	osName := strings.TrimSpace(c.QueryParam("os"))
	arch := strings.TrimSpace(c.QueryParam("arch"))
	if osName == "" || arch == "" {
		return c.JSON(400, _type.H{Code: "error", Msg: "os and arch are required"})
	}

	meta, err := updateproxy.GetLatest(osName, arch)
	if err != nil {
		if _, vErr := githubrelease.AssetNameForPlatform(osName, arch); vErr != nil {
			return c.JSON(400, _type.H{Code: "error", Msg: err.Error()})
		}
		if strings.Contains(err.Error(), "not ready") {
			return c.JSON(503, _type.H{Code: "error", Msg: "release metadata not available"})
		}
		return c.JSON(404, _type.H{Code: "error", Msg: "platform release not found"})
	}

	downloadQuery := url.Values{
		"os":   []string{osName},
		"arch": []string{arch},
	}.Encode()
	downloadURL := "/api/agent/update/download?" + downloadQuery

	return c.JSON(200, _type.H{
		Code: "ok",
		Data: updateLatestData{
			ReleaseTag:  meta.ReleaseTag,
			AssetName:   meta.AssetName,
			SHA256:      meta.SHA256,
			DownloadURL: downloadURL,
			CachedAt:    meta.CachedAt.Format("2006-01-02T15:04:05Z"),
		},
	})
}

func updateDownload(c *echo.Context) error {
	osName := strings.TrimSpace(c.QueryParam("os"))
	arch := strings.TrimSpace(c.QueryParam("arch"))
	if osName == "" || arch == "" {
		return c.JSON(400, _type.H{Code: "error", Msg: "os and arch are required"})
	}

	upstream, assetName, err := updateproxy.GetGitHubDownloadURL(osName, arch)
	if err != nil {
		if _, vErr := githubrelease.AssetNameForPlatform(osName, arch); vErr != nil {
			return c.JSON(400, _type.H{Code: "error", Msg: err.Error()})
		}
		if strings.Contains(err.Error(), "not ready") {
			return c.JSON(503, _type.H{Code: "error", Msg: "release metadata not available"})
		}
		return c.JSON(404, _type.H{Code: "error", Msg: "platform release not found"})
	}

	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, upstream, nil)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "failed to build upstream request"})
	}
	req.Header.Set("User-Agent", "mosona-manager-hub")
	resp, err := githubrelease.NewClient().Do(req)
	if err != nil {
		return c.JSON(502, _type.H{Code: "error", Msg: "failed to reach release host"})
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return c.JSON(502, _type.H{Code: "error", Msg: fmt.Sprintf("upstream returned %d", resp.StatusCode)})
	}

	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", assetName))
	c.Response().WriteHeader(http.StatusOK)
	_, err = io.Copy(c.Response(), resp.Body)
	return err
}
