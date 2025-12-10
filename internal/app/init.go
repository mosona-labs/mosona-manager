package app

import (
	"context"
	"fmt"
	"log"
	"mosona-manager/internal/app/api/admin"
	"mosona-manager/internal/app/api/category"
	"mosona-manager/internal/app/api/keys"
	"mosona-manager/internal/app/api/logs"
	"mosona-manager/internal/app/api/server"
	"mosona-manager/internal/app/api/team"
	"mosona-manager/internal/app/api/user"
	"mosona-manager/internal/app/auth"
	middleware2 "mosona-manager/internal/app/middleware"
	"mosona-manager/internal/config"
	"mosona-manager/internal/redis"
	"mosona-manager/pkg/_type"

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
	e.Static("/static", "./static")
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
		alogs.Router(v1.Group("/logs"))         // Logs
	}
	// Admin
	admin.Router(api.Group("/admin", middleware2.AdminAuth))

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
