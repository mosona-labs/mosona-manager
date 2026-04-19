package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/app/agent"
	"mosona-manager/internal/app/api/admin"
	"mosona-manager/internal/app/api/category"
	"mosona-manager/internal/app/api/keys"
	"mosona-manager/internal/app/api/logs"
	apublic "mosona-manager/internal/app/api/public"
	"mosona-manager/internal/app/api/server"
	aalert "mosona-manager/internal/app/api/server/alert"
	"mosona-manager/internal/app/api/team"
	"mosona-manager/internal/app/api/user"
	"mosona-manager/internal/app/auth"
	init2 "mosona-manager/internal/app/init"
	middleware2 "mosona-manager/internal/app/middleware"
	"mosona-manager/internal/config"
	"mosona-manager/internal/redis"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/rbcervilla/redisstore/v9"
)

func Start() {
	e := echo.New()
	e.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))

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
	store.Options(sessions.Options{
		Path:   "/",
		MaxAge: 43200,
	})
	e.Use(session.Middleware(store))
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
		auth.Router(api.Group("/auth"))
		apublic.Router(api.Group("/public"))

		// PING
		api.GET("/ping", func(c *echo.Context) error {
			return c.String(200, "pong!")
		})
	}
	v1 := api.Group("/v1", middleware2.UserAuth, middleware2.UserRole)
	{
		auser.Router(v1.Group("/user"))         // User
		ateam.Router(v1.Group("/team"))         // Team
		akeys.Router(v1.Group("/key"))          // Keys
		acategory.Router(v1.Group("/category")) // Category
		aserver.Router(v1.Group("/server"))     // Server
		aalert.Router(v1.Group("/alert"))       // Alert
		alogs.Router(v1.Group("/logs"))         // Logs
	}
	// Admin
	admin.Router(api.Group("/admin", middleware2.AdminAuth))
	// Agent
	agent.Router(api.Group("/agent"))
	// Init
	init2.Router(api.Group("/init"))

	// Health
	e.GET("/health", func(c *echo.Context) error {
		return c.JSON(200, _type.H{
			Code: "ok",
			Msg:  "Service is healthy",
		})
	})

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
		fsPath := filepath.Join(config.Conf.FrontendDir, path.Clean(reqPath))
		if fi, err := os.Stat(fsPath); err == nil && !fi.IsDir() {
			relativePath := strings.TrimPrefix(path.Clean("/"+reqPath), "/")
			return c.FileFS(relativePath, frontendFS)
		}
		return c.FileFS("index.html", frontendFS)
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
	sc := echo.StartConfig{
		Address:         address,
		HideBanner:      true,
		HidePort:        true,
		GracefulTimeout: 3 * time.Second,
	}
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("failed to start or shutdown server", "error", err)
	}
	e.Logger.Info("Server stopped gracefully")
}

func HealthCheck() error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(fmt.Sprintf(
		"http://%s:%d/health",
		config.Conf.Host, config.Conf.Port,
	))
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
