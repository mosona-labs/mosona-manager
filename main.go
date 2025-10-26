package main

import (
	"fmt"
	"log"
	"mosona-manager/app"
	"mosona-manager/db"
	"mosona-manager/influx"
	"mosona-manager/redis"
	"os"
)

const version = "v0.0.01"
const Logo = `┳┳┓           ┳┳┓            
┃┃┃┏┓┏┏┓┏┓┏┓  ┃┃┃┏┓┏┓┏┓┏┓┏┓┏┓
┛ ┗┗┛┛┗┛┛┗┗┻  ┛ ┗┗┻┛┗┗┻┗┫┗ ┛ 
                        ┛`

func main() {
	fmt.Println(Logo)
	fmt.Println("⇨ Mosona manager " + version + " starting...")

	db.Init()     // Postgres
	influx.Init() // InfluxDB
	redis.Init()  // Redis

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
