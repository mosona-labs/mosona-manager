package apublic

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

const (
	publicPageContextKey     = "public_page"
	publicPageResolveTimeout = 3 * time.Second
)

var publicPageResolveSlots = make(chan struct{}, 8)

var (
	styleCloseTagPattern = regexp.MustCompile(`(?i)</style`)
	faviconLinkPattern   = regexp.MustCompile(`(?i)<link\b[^>]*\brel=["'](?:shortcut\s+)?icon["'][^>]*>`)
)

func PageByName(c *echo.Context) error {
	page, err := resolvePageByName(c.Request().Context(), c.Param("name"))
	if err != nil {
		return publicResolveError(c, err)
	}

	setPublicPageHeaders(c)
	return servePreviewIndex(c, &page)
}

func TryServeDomainRequest(c *echo.Context) (bool, error) {
	page, err := resolvePageByDomainHost(c.Request().Context(), c.Request().Host)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	setPublicPageHeaders(c)

	relativePath := strings.TrimPrefix(path.Clean("/"+c.Request().URL.Path), "/")
	if relativePath != "" {
		if fi, statErr := fs.Stat(frontendFS(), relativePath); statErr == nil && !fi.IsDir() {
			return true, c.FileFS(relativePath, frontendFS())
		}
	}

	return true, servePreviewIndex(c, &page)
}

func bootstrap(c *echo.Context) error {
	page, ok := c.Get(publicPageContextKey).(*_type.ResolvedPublicPage)
	if !ok || page == nil {
		return publicResolveError(c, sql.ErrNoRows)
	}

	snapshot, err := publicSnapshots.get(c.Request().Context(), page.TeamID)
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
			"servers":    snapshot.Servers,
			"categories": snapshot.Categories,
			"status":     snapshot.Status,
			"now":        snapshot.Now,
		},
	})
}

func resolvePublicPageByName(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		page, err := resolvePageByName(c.Request().Context(), c.Param("name"))
		if err != nil {
			return publicResolveError(c, err)
		}
		c.Set(publicPageContextKey, &page)
		return next(c)
	}
}

func resolvePublicPageByDomain(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		page, err := resolvePageByDomainHost(c.Request().Context(), c.Request().Host)
		if err != nil {
			return publicResolveError(c, err)
		}
		c.Set(publicPageContextKey, &page)
		return next(c)
	}
}

func resolvePageByName(ctx context.Context, name string) (_type.ResolvedPublicPage, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return _type.ResolvedPublicPage{}, sql.ErrNoRows
	}
	return resolvePublicPage(ctx, func(queryCtx context.Context) (_type.ResolvedPublicPage, error) {
		return db.GetEnabledTeamPublicPageByNameContext(queryCtx, name)
	})
}

func resolvePageByDomainHost(ctx context.Context, host string) (_type.ResolvedPublicPage, error) {
	host = normalizeHost(host)
	if host == "" {
		return _type.ResolvedPublicPage{}, sql.ErrNoRows
	}
	return resolvePublicPage(ctx, func(queryCtx context.Context) (_type.ResolvedPublicPage, error) {
		return db.GetEnabledTeamPublicPageByDomainContext(queryCtx, host)
	})
}

func resolvePublicPage(
	ctx context.Context,
	load func(context.Context) (_type.ResolvedPublicPage, error),
) (_type.ResolvedPublicPage, error) {
	select {
	case publicPageResolveSlots <- struct{}{}:
		defer func() { <-publicPageResolveSlots }()
	case <-ctx.Done():
		return _type.ResolvedPublicPage{}, ctx.Err()
	default:
		return _type.ResolvedPublicPage{}, errors.New("public page resolver is busy")
	}
	queryCtx, cancel := context.WithTimeout(ctx, publicPageResolveTimeout)
	defer cancel()
	return load(queryCtx)
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
