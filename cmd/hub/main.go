package main

import (
	"fmt"
	"log"
	"mosona-manager/internal/app"
	"mosona-manager/internal/connect"
	db2 "mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/oauth"
	"mosona-manager/internal/redis"
	"mosona-manager/internal/task"
	"os"
)

const version = "v0.0.1"
const Logo = `┳┳┓           ┳┳┓            
┃┃┃┏┓┏┏┓┏┓┏┓  ┃┃┃┏┓┏┓┏┓┏┓┏┓┏┓
┛ ┗┗┛┛┗┛┛┗┗┻  ┛ ┗┗┻┛┗┗┻┗┫┗ ┛ 
                        ┛`

func main() {
	fmt.Println(Logo)
	fmt.Println("⇨ Mosona manager " + version + " starting...")

	// Database
	db2.Init()    // Postgres
	influx.Init() // InfluxDB
	redis.Init()  // Redis
	// Dynamic Config
	if err := db2.SyncConfig(); err != nil {
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
