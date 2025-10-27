package app

import (
	"context"
	"fmt"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rbcervilla/redisstore/v9"
	"log"
	"mosona-manager/_type"
	ateam "mosona-manager/app/api/team"
	auser "mosona-manager/app/api/user"
	"mosona-manager/app/auth"
	"mosona-manager/config"
	"mosona-manager/redis"

	inMiddleware "mosona-manager/app/middleware"
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
	v1 := api.Group("/v1", inMiddleware.UserAuth)
	{
		auser.Router(v1.Group("/user")) // User
		ateam.Router(v1.Group("/team")) // Team
	}

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
