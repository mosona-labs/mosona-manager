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
	"mosona-manager/app/api/auth"
	"mosona-manager/config"
	"mosona-manager/redis"

	//inMiddleware "mosona-manager/app/middleware"
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

	// API Routers
	api := e.Group("/api")
	{
		auth.Router(api.Group("/auth"))
	}
	//v1 := api.Group("/v1", inMiddleware.UserAuth)
	//{
	//	// More routers can be added here
	//}

	// Start
	e.Logger.Fatal(e.Start(fmt.Sprintf(
		"%s:%d",
		config.Conf.Host, config.Conf.Port,
	)))
}
