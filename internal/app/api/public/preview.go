package apublic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

const publicPageContextKey = "public_page"

var (
	styleCloseTagPattern = regexp.MustCompile(`(?i)</style`)
	faviconLinkPattern   = regexp.MustCompile(`(?i)<link\b[^>]*\brel=["'](?:shortcut\s+)?icon["'][^>]*>`)
)

func PageByName(c *echo.Context) error {
	page, err := resolvePageByName(c.Param("name"))
	if err != nil {
		return publicResolveError(c, err)
	}

	setPublicPageHeaders(c)
	return servePreviewIndex(c, &page)
}

func TryServeDomainRequest(c *echo.Context) (bool, error) {
	page, err := resolvePageByDomainHost(c.Request().Host)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	setPublicPageHeaders(c)
	return true, servePreviewIndex(c, &page)
}

func bootstrap(c *echo.Context) error {
	page, ok := c.Get(publicPageContextKey).(*_type.ResolvedPublicPage)
	if !ok || page == nil {
		return publicResolveError(c, sql.ErrNoRows)
	}

	servers, categories, statusMap, now, err := loadPublicSnapshot(page.TeamID)
	if err != nil {
		setPublicPageHeaders(c)
		return utils.ErrorHandler(c, err, "Failed to load public preview data")
	}

	setPublicPageHeaders(c)
	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: _type.Map{
			"page":       buildPublicPageSummary(page),
			"servers":    servers,
			"categories": categories,
			"status":     statusMap,
			"now":        now,
		},
	})
}

func sse(c *echo.Context) error {
	page, ok := c.Get(publicPageContextKey).(*_type.ResolvedPublicPage)
	if !ok || page == nil {
		return publicResolveError(c, sql.ErrNoRows)
	}

	setPublicPageHeaders(c)
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(200)

	ctx, cancel := context.WithCancel(c.Request().Context())
	defer cancel()

	sendData := func() {
		servers, categories, statusMap, now, err := loadPublicSnapshot(page.TeamID)
		if err != nil {
			_, _ = fmt.Fprintf(c.Response(), "event: error\ndata: {\"msg\":\"Failed to load public preview data\"}\n\n")
			if flusher, ok := c.Response().(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}

		data, err := json.Marshal(_type.Map{
			"servers":    servers,
			"categories": categories,
			"status":     statusMap,
			"now":        now,
		})
		if err != nil {
			_, _ = fmt.Fprintf(c.Response(), "event: error\ndata: {\"msg\":\"Failed to encode public preview data\"}\n\n")
			if flusher, ok := c.Response().(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}

		_, _ = fmt.Fprintf(c.Response(), "event: update\ndata: %s\n\n", string(data))
		if flusher, ok := c.Response().(http.Flusher); ok {
			flusher.Flush()
		}
	}

	sendData()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			sendData()
		}
	}
}

func resolvePublicPageByName(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		page, err := resolvePageByName(c.Param("name"))
		if err != nil {
			return publicResolveError(c, err)
		}
		c.Set(publicPageContextKey, &page)
		return next(c)
	}
}

func resolvePublicPageByDomain(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		page, err := resolvePageByDomainHost(c.Request().Host)
		if err != nil {
			return publicResolveError(c, err)
		}
		c.Set(publicPageContextKey, &page)
		return next(c)
	}
}

func resolvePageByName(name string) (_type.ResolvedPublicPage, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return _type.ResolvedPublicPage{}, sql.ErrNoRows
	}
	return db.GetEnabledTeamPublicPageByName(name)
}

func resolvePageByDomainHost(host string) (_type.ResolvedPublicPage, error) {
	host = normalizeHost(host)
	if host == "" {
		return _type.ResolvedPublicPage{}, sql.ErrNoRows
	}
	return db.GetEnabledTeamPublicPageByDomain(host)
}

func loadPublicSnapshot(teamID int64) ([]_type.PublicMonitor, []_type.Category, map[int64]*_type.ServerStatusType, int64, error) {
	servers, err := db.ListPublicMonitoredServers(teamID)
	if err != nil {
		return nil, nil, nil, 0, err
	}

	categories, err := db.GetCategoriesByTeam(teamID)
	if err != nil {
		return nil, nil, nil, 0, err
	}

	ids := make([]int64, 0, len(servers))
	for _, server := range servers {
		ids = append(ids, server.ID)
	}

	statusMap, err := influx.GetLatestServerStatusBatch(ids)
	if err != nil {
		return nil, nil, nil, 0, err
	}

	return servers, categories, statusMap, time.Now().Unix(), nil
}

func publicResolveError(c *echo.Context, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		setPublicPageHeaders(c)
		return c.JSON(404, _type.H{
			Code: "not_found",
			Msg:  "Public page not found",
		})
	}

	setPublicPageHeaders(c)
	return utils.ErrorHandler(c, err, "Failed to resolve public page")
}

func publicPageTitle(page *_type.ResolvedPublicPage) string {
	if page.Title != nil && strings.TrimSpace(*page.Title) != "" {
		return strings.TrimSpace(*page.Title)
	}
	return page.TeamName + " Status"
}

func buildPublicPageSummary(page *_type.ResolvedPublicPage) _type.PublicPageSummary {
	summary := _type.PublicPageSummary{
		Title:       publicPageTitle(page),
		Name:        page.Name,
		Domain:      page.Domain,
		Description: page.Description,
		CustomCSS:   page.CustomCSS,
		TeamName:    page.TeamName,
		TeamColor:   page.TeamColor,
		TeamImage:   page.TeamImage,
	}
	if page.TeamImage != "" {
		avatar := publicAvatarURL(page.TeamImage)
		if avatar != "" {
			summary.TeamAvatar = &avatar
		}
	}
	return summary
}

func previewFS() fs.FS {
	return echo.NewDefaultFS(filepath.Join(config.Conf.FrontendDir, "public-preview"))
}

func frontendFS() fs.FS {
	return echo.NewDefaultFS(config.Conf.FrontendDir)
}

func servePreviewIndex(c *echo.Context, page *_type.ResolvedPublicPage) error {
	index, err := fs.ReadFile(previewFS(), "index.html")
	if err != nil {
		return err
	}

	html := applyPreviewFavicon(string(index), page)
	css := ""
	if page != nil && page.CustomCSS != nil {
		css = strings.TrimSpace(*page.CustomCSS)
	}
	if css == "" {
		return c.HTML(200, html)
	}

	style := "<style id=\"mosona-public-page-custom-css\">\n" + sanitizeStyleContent(css) + "\n</style>\n"
	withStyle := strings.Replace(html, "</head>", style+"</head>", 1)
	if withStyle == html {
		withStyle = style + html
	}
	return c.HTML(200, withStyle)
}

func sanitizeStyleContent(css string) string {
	return styleCloseTagPattern.ReplaceAllString(css, `<\/style`)
}

func applyPreviewFavicon(html string, page *_type.ResolvedPublicPage) string {
	if page == nil || strings.TrimSpace(page.TeamImage) == "" {
		return html
	}

	href := publicAvatarURL(page.TeamImage)
	if href == "" {
		return html
	}

	link := `<link rel="icon" href="` + href + `" />`
	if faviconLinkPattern.MatchString(html) {
		replaced := false
		return faviconLinkPattern.ReplaceAllStringFunc(html, func(match string) string {
			if replaced {
				return ""
			}
			replaced = true
			return link
		})
	}
	if strings.Contains(html, "</head>") {
		return strings.Replace(html, "</head>", link+"\n</head>", 1)
	}
	return link + "\n" + html
}

func publicAvatarURL(image string) string {
	image = path.Base(strings.TrimSpace(image))
	if image == "" || image == "." || image == "/" {
		return ""
	}
	return "/avatars/" + url.PathEscape(image)
}

func setPublicPageHeaders(c *echo.Context) {
	c.Response().Header().Set("X-Robots-Tag", "noindex")
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}

	if value, _, err := net.SplitHostPort(host); err == nil {
		host = value
	}

	return strings.Trim(host, "[]")
}
