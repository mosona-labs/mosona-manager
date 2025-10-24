package config

type configType struct {
	Host string
	Port int

	// Postgres
	PostgresHost string
	PostgresPort int
	PostgresUser string
	PostgresPass string
	PostgresDB   string

	// InfluxDB 2
	InfluxDBUrl    string
	InfluxDBOrg    string
	InfluxDBBucket string
	InfluxDBToken  string

	// Redis
	RedisHost     string
	RedisPort     int
	RedisPassword string
}

type dynamicConfigType struct {
	Init bool

	// Salt & Token
	Token string

	// Captcha
	CaptchaSecret  string
	CaptchaSiteKey string

	// Google OAuth
	GoogleClientID     string
	GoogleClientSecret string
}
