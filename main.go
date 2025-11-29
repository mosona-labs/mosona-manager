package main

import (
	"fmt"
	"log"
	"mosona-manager/app"
	"mosona-manager/connect"
	"mosona-manager/db"
	"mosona-manager/influx"
	"mosona-manager/redis"
	"mosona-manager/task"
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

	db.Init()      // Postgres
	influx.Init()  // InfluxDB
	redis.Init()   // Redis
	connect.Init() // SSH Connect

	task.Init() // Background Task

	// Sync
	if err := db.SyncConfig(); err != nil {
		log.Fatalln("Sync config error:", err)
	}
	// Dir
	if err := os.MkdirAll("./avatars", os.ModePerm); err != nil {
		log.Fatalln("Create avatars dir error:", err)
	}

	// API
	app.Start()
}
