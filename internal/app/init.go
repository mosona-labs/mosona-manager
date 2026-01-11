package app

import (
	"context"
	"fmt"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/app/agent"
	"mosona-manager/internal/app/api/admin"
	"mosona-manager/internal/app/api/category"
	"mosona-manager/internal/app/api/keys"
	"mosona-manager/internal/app/api/logs"
	"mosona-manager/internal/app/api/server"
	aalert "mosona-manager/internal/app/api/server/alert"
	"mosona-manager/internal/app/api/team"
	"mosona-manager/internal/app/api/user"
	"mosona-manager/internal/app/auth"
	middleware2 "mosona-manager/internal/app/middleware"
	"mosona-manager/internal/config"
	"mosona-manager/internal/redis"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rbcervilla/redisstore/v9"
)

func Start() {
	e := echo.New()
	e.HideBanner = true

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

		// PING
		api.GET("/ping", func(c echo.Context) error {
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

	// Health
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, _type.H{
			Code: "ok",
			Msg:  "Service is healthy",
		})
	})

	// Frontend
	e.GET("/*", func(c echo.Context) error {
		reqPath := c.Request().URL.Path
		fsPath := filepath.Join(config.Conf.FrontendDir, path.Clean(reqPath))
		if fi, err := os.Stat(fsPath); err == nil && !fi.IsDir() {
			return c.File(fsPath)
		}
		return c.File(filepath.Join(config.Conf.FrontendDir, "index.html"))
	})

	// NotFound
	api.Any("/*", func(c echo.Context) error {
		return c.JSON(404, _type.H{
			Code: "not_found",
			Msg:  "API not found",
		})
	})

	// Start
	e.Logger.Fatal(e.Start(fmt.Sprintf(
		"%s:%d",
		config.Conf.Host, config.Conf.Port,
	)))
}

func HealthCheck() error {
	resp, err := http.Get(fmt.Sprintf(
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
