package main

import (
	"fmt"
	"log"
	"mosona-manager/internal/app"
	"mosona-manager/internal/connect"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/oauth"
	"mosona-manager/internal/redis"
	"mosona-manager/internal/runtime"
	"mosona-manager/internal/task"
	"os"
)

const Logo = `┳┳┓           ┳┳┓            
┃┃┃┏┓┏┏┓┏┓┏┓  ┃┃┃┏┓┏┓┏┓┏┓┏┓┏┓
┛ ┗┗┛┛┗┛┛┗┗┻  ┛ ┗┗┻┛┗┗┻┗┫┗ ┛ 
                        ┛`

func main() {
	fmt.Println(Logo)
	fmt.Println("⇨ Mosona manager v" + runtime.Version + " starting...")

	// Database
	db.Init()     // Postgres
	influx.Init() // InfluxDB
	redis.Init()  // Redis
	// Dynamic Config
	if err := db.SyncConfig(); err != nil {
		log.Fatalln("Sync config error:", err)
	}

	// OAuth
	oauth.Init()

	// SSH Connect
	connect.Init()

	// Other Task
	task.Init()

	// Dir
	if err := os.MkdirAll("./avatars", os.ModePerm); err != nil {
		log.Fatalln("Create avatars dir error:", err)
	}

	// API Server
	app.Start()
}
