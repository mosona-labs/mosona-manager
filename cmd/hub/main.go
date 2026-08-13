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
	"mosona-manager/internal/siteaccess"
	"mosona-manager/internal/task"
	"mosona-manager/internal/utils/encrypt"
	"os"
)

const Logo = `┳┳┓           ┳┳┓            
┃┃┃┏┓┏┏┓┏┓┏┓  ┃┃┃┏┓┏┓┏┓┏┓┏┓┏┓
┛ ┗┗┛┛┗┛┛┗┗┻  ┛ ┗┗┻┛┗┗┻┗┫┗ ┛ 
                        ┛`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "health":
			if err := app.HealthCheck(); err != nil {
				log.Fatalln("Health check failed:", err)
			}
			fmt.Println("ok")
			return
		}
	}

	initApp()
}

func initApp() {
	// Start
	fmt.Println(Logo)
	fmt.Println("⇨ Mosona manager v" + runtime.Version + " starting...")

	// Database
	db.Init() // Postgres
	encryptedCredentialsExist, err := db.HasEncryptedCredentials()
	if err != nil {
		log.Fatalln("Check encrypted credentials error:", err)
	}
	keyPath, err := encrypt.Initialize(encryptedCredentialsExist)
	if err != nil {
		log.Fatalln("Initialize encryption key error:", err)
	}
	log.Printf("Encryption key loaded from %s", keyPath)
	migrationReport, err := db.MigrateEncryptedCredentials()
	if err != nil {
		log.Fatalln("Migrate encrypted credentials error:", err)
	}
	for _, failure := range migrationReport.Failures {
		log.Printf(
			"WARNING: skipped unreadable encrypted credential table=%s record_id=%d field=%s error=%v; replace or delete this credential",
			failure.Table, failure.RecordID, failure.Field, failure.Err,
		)
	}
	influx.Init() // InfluxDB
	redis.Init()  // Redis
	// Dynamic Config
	if err := db.SyncConfig(); err != nil {
		log.Fatalln("Sync config error:", err)
	}
	if err := siteaccess.Refresh(); err != nil {
		log.Fatalln("Site access cache error:", err)
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
