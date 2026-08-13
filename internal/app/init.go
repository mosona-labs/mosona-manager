package app

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"log"
	"log/slog"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/app/agent"
	"mosona-manager/internal/app/api/admin"
	acategory "mosona-manager/internal/app/api/category"
	akeys "mosona-manager/internal/app/api/keys"
	alogs "mosona-manager/internal/app/api/logs"
	apublic "mosona-manager/internal/app/api/public"
	aserver "mosona-manager/internal/app/api/server"
	aalert "mosona-manager/internal/app/api/server/alert"
	ateam "mosona-manager/internal/app/api/team"
	auser "mosona-manager/internal/app/api/user"
	"mosona-manager/internal/app/auth"
	init2 "mosona-manager/internal/app/init"
	middleware2 "mosona-manager/internal/app/middleware"
	"mosona-manager/internal/config"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/redis"
	"mosona-manager/internal/runtime"
	"mosona-manager/internal/utils"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/rbcervilla/redisstore/v9"
)

var (
	siteTitlePattern       = regexp.MustCompile(`(?is)<title\b[^>]*>.*?</title>`)
	siteFaviconLinkPattern = regexp.MustCompile(`(?i)<link\b[^>]*\brel=["'](?:shortcut\s+)?icon["'][^>]*>`)
)

func Start() {
	e := echo.New()
	e.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	configureClientIPExtractor(e)

	address := fmt.Sprintf("%s:%d", config.Conf.Host, config.Conf.Port)

	// CTX
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// SESSION
	store, err := redisstore.NewRedisStore(context.Background(), redis.Client)
	if err != nil {
		log.Fatalln("Init Redis:", err)
	}
	store.KeyPrefix("mosona:session:")
	store.Options(*auth.StoreOptions(43200))
	e.Use(session.Middleware(store))
	e.Use(middleware2.RequireUserBaseHost, middleware2.RestrictPublicPageHost)
	// GZIP
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5,
	}))
	// Decompress
	e.Use(middleware.Decompress())

	// Static
	e.Static("/avatars", "./avatars")

	// API Routers
	api := e.Group("/api")
	{
		auth.Router(api.Group("/auth", middleware2.SameOriginWrite))
		apublic.Router(api.Group("/public"))

		// PING
		api.GET("/ping", func(c *echo.Context) error {
			return c.String(200, "pong!")
		})
	}
	v1 := api.Group("/v1", middleware2.SameOriginWrite, middleware2.UserAuth)
	{
		auser.Router(v1.Group("/user")) // User
		ateam.Router(v1.Group("/team")) // Team (mixed user/team routes)
		akeys.Router(v1.Group("/key", middleware2.TeamAccess))
		acategory.Router(v1.Group("/category", middleware2.TeamAccess))
		aserver.Router(v1.Group("/server", middleware2.TeamAccess))
		aalert.Router(v1.Group("/alert", middleware2.TeamAccess))
		alogs.Router(v1.Group("/logs", middleware2.TeamAccess))

		// Version
		v1.GET("/version", func(c *echo.Context) error {
			return c.JSON(200, _type.Map{
				"code":    "ok",
				"version": runtime.Version,
			})
		})
	}
	// Admin
	admin.Router(api.Group("/admin", middleware2.SameOriginWrite, middleware2.AdminAuth))
	// Agent
	agent.Router(api.Group("/agent"))
	// Init
	init2.Router(api.Group("/init", middleware2.SameOriginWrite))

	registerHealthRoutes(e)

	// Public preview
	e.Static("/preview-assets", filepath.Join(config.Conf.FrontendDir, "public-preview"))
	frontendFS := echo.NewDefaultFS(config.Conf.FrontendDir)
	e.GET("/preview/:name", apublic.PageByName)

	// Frontend
	frontendHandler := func(c *echo.Context) error {
		if served, err := apublic.TryServeDomainRequest(c); err != nil {
			return err
		} else if served {
			return nil
		}

		reqPath := c.Request().URL.Path
		fsPath, err := utils.SafeJoinUnderRoot(config.Conf.FrontendDir, reqPath)
		if err != nil {
			return serveFrontendIndex(c, frontendFS)
		}
		if fi, err := os.Stat(fsPath); err == nil && !fi.IsDir() {
			relativePath := strings.TrimPrefix(path.Clean("/"+reqPath), "/")
			return c.FileFS(relativePath, frontendFS)
		}
		return serveFrontendIndex(c, frontendFS)
	}
	e.GET("/", frontendHandler)
	e.GET("/*", frontendHandler)

	// NotFound
	api.Any("/*", func(c *echo.Context) error {
		return c.JSON(404, _type.H{
			Code: "not_found",
			Msg:  "API not found",
		})
	})

	// Start
	fmt.Println("⇨ Listening on http://" + address)
	initializationComplete.Store(true)
	defer initializationComplete.Store(false)
	sc := echo.StartConfig{
		Address:         address,
		HideBanner:      true,
		HidePort:        true,
		GracefulTimeout: 3 * time.Second,
	}
	startErr := sc.Start(ctx, e)
	shutdownInflux()
	if startErr != nil && !errors.Is(startErr, http.ErrServerClosed) {
		log.Fatal("failed to start or shutdown server", "error", startErr)
	}
	e.Logger.Info("Server stopped gracefully")
}

func shutdownInflux() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := influx.ShutdownAuditLogs(ctx); err != nil {
		stats := influx.CurrentAuditLogQueueStats()
		log.Printf("Audit log shutdown did not drain fully (written=%d failed=%d dropped=%d): %v", stats.Written, stats.Failed, stats.Dropped, err)
		return
	}
	if influx.Client != nil {
		influx.Client.Close()
	}
}

func serveFrontendIndex(c *echo.Context, frontendFS fs.FS) error {
	index, err := fs.ReadFile(frontendFS, "index.html")
	if err != nil {
		return err
	}
	return c.HTML(http.StatusOK, applySiteMetadata(string(index)))
}

func applySiteMetadata(htmlDoc string) string {
	dc := config.ReadDynamicConf()

	if title := strings.TrimSpace(dc.Title); title != "" {
		titleTag := "<title>" + html.EscapeString(title) + "</title>"
		if siteTitlePattern.MatchString(htmlDoc) {
			htmlDoc = siteTitlePattern.ReplaceAllString(htmlDoc, titleTag)
		} else if strings.Contains(htmlDoc, "</head>") {
			htmlDoc = strings.Replace(htmlDoc, "</head>", titleTag+"\n</head>", 1)
		} else {
			htmlDoc = titleTag + "\n" + htmlDoc
		}
	}

	if favicon := siteFaviconURL(dc.Favicon); favicon != "" {
		link := `<link rel="icon" href="` + html.EscapeString(favicon) + `" />`
		if siteFaviconLinkPattern.MatchString(htmlDoc) {
			replaced := false
			htmlDoc = siteFaviconLinkPattern.ReplaceAllStringFunc(htmlDoc, func(match string) string {
				if replaced {
					return ""
				}
				replaced = true
				return link
			})
		} else if strings.Contains(htmlDoc, "</head>") {
			htmlDoc = strings.Replace(htmlDoc, "</head>", link+"\n</head>", 1)
		} else {
			htmlDoc = link + "\n" + htmlDoc
		}
	}

	return htmlDoc
}

func siteFaviconURL(favicon string) string {
	favicon = strings.TrimSpace(favicon)
	if favicon == "" {
		return ""
	}
	if strings.HasPrefix(favicon, "/") && !strings.HasPrefix(favicon, "//") {
		return favicon
	}

	name := path.Base(favicon)
	if name == "" || name == "." || name == "/" {
		return ""
	}
	return "/avatars/" + url.PathEscape(name)
}

func HealthCheck() error {
	client := &http.Client{Timeout: healthCheckClientTimeout}
	return runHealthCheck(client, healthCheckURL(config.Conf.Host, config.Conf.Port))
}

func runHealthCheck(client *http.Client, healthURL string) error {
	resp, err := client.Get(healthURL)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != 200 {
		return fmt.Errorf("health check failed with status code: %d", resp.StatusCode)
	}
	return nil
}

func healthCheckURL(host string, port int) string {
	address := net.JoinHostPort(healthCheckHost(host), strconv.Itoa(port))
	return "http://" + address + "/health/ready"
}

func healthCheckHost(host string) string {
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return strings.Trim(host, "[]")
	}
}
