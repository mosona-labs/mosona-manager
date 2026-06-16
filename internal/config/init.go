package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

var Conf configType

func init() {
	_ = godotenv.Load(".env")

	Conf.Host = getEnv("HOST", "0.0.0.0")
	Conf.Port, _ = strconv.Atoi(getEnv("PORT", "0"))
	if Conf.Port == 0 {
		Conf.Port = 3214
	}

	// Postgres
	Conf.PostgresHost = getEnv("POSTGRES_HOST", "localhost")
	Conf.PostgresPort, _ = strconv.Atoi(getEnv("POSTGRES_PORT", "0"))
	Conf.PostgresUser = getEnv("POSTGRES_USER", "")
	Conf.PostgresPass = getEnv("POSTGRES_PASS", "")
	Conf.PostgresDB = getEnv("POSTGRES_DB", "")
	if Conf.PostgresPort == 0 || Conf.PostgresUser == "" || Conf.PostgresPass == "" || Conf.PostgresDB == "" {
		log.Fatalln("Postgres configuration is missing in environment variables")
	}

	// InfluxDB 2
	Conf.InfluxDBUrl = getEnv("INFLUXDB_URL", "http://localhost:8086")
	Conf.InfluxDBOrg = getEnv("INFLUXDB_ORG", "")
	Conf.InfluxDBToken = getEnv("INFLUXDB_TOKEN", "")
	if Conf.InfluxDBOrg == "" || Conf.InfluxDBToken == "" {
		log.Fatalln("InfluxDB configuration is missing in environment variables")
	}

	// Redis
	Conf.RedisHost = getEnv("REDIS_HOST", "localhost")
	Conf.RedisPort, _ = strconv.Atoi(getEnv("REDIS_PORT", "0"))
	Conf.RedisPassword = getEnv("REDIS_PASSWORD", "")
	if Conf.RedisPort == 0 {
		log.Fatalln("Redis configuration is missing in environment variables")
	}

	// Frontend
	Conf.FrontendDir = getEnv("FRONTEND_DIR", "./static/")

	Conf.TrustProxy = getEnv("TRUST_PROXY", "") == "true"
}

func getEnv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
