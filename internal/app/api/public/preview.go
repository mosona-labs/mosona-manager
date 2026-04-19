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
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

const publicPageContextKey = "public_page"

func PageByName(c *echo.Context) error {
	_, err := resolvePageByName(c.Param("name"))
	if err != nil {
		return publicResolveError(c, err)
	}

	setPublicPageHeaders(c)
	return c.FileFS("index.html", previewFS())
}

func TryServeDomainPage(c *echo.Context) (bool, error) {
	_, err := resolvePageByDomainHost(c.Request().Host)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	setPublicPageHeaders(c)
	return true, c.FileFS("index.html", previewFS())
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
		TeamName:    page.TeamName,
		TeamColor:   page.TeamColor,
		TeamImage:   page.TeamImage,
	}
	if page.TeamImage != "" {
		avatar := "/avatars/" + page.TeamImage
		summary.TeamAvatar = &avatar
	}
	return summary
}

func previewFS() fs.FS {
	return echo.NewDefaultFS(filepath.Join(config.Conf.FrontendDir, "public-preview"))
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
