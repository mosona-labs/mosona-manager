package db

import (
	"fmt"
	"log"
	"mosona-manager/config"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var Db *sqlx.DB

func Init() {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.Conf.PostgresHost,
		config.Conf.PostgresPort,
		config.Conf.PostgresUser,
		config.Conf.PostgresPass,
		config.Conf.PostgresDB,
	)

	var err error
	Db, err = sqlx.Open("postgres", dsn)
	if err != nil {
		log.Fatalln("Failed to connect to Postgres:", err)
	}

	Db.SetMaxOpenConns(50)
	Db.SetMaxIdleConns(25)
	Db.SetConnMaxLifetime(30 * time.Minute)

	for i := 0; i < 5; i++ {
		if err = Db.Ping(); err == nil {
			return
		}
		log.Printf("Postgres ping failed (attempt %d): %v", i+1, err)
		time.Sleep(time.Duration(i+1) * time.Second)
	}

	log.Fatalln("Failed to connect to Postgres after retries:", err)
}
