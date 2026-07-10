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
	InfluxDBUrl   string
	InfluxDBOrg   string
	InfluxDBToken string

	// Redis
	RedisHost     string
	RedisPort     int
	RedisPassword string

	// Frontend
	FrontendDir string
}

type DynamicConfigType struct {
	Init  bool
	Debug bool

	// Site
	Title   string
	Favicon string

	// Domain
	Domain string

	// Salt & Token
	Token string

	// Captcha
	CaptchaSecret  string
	CaptchaSiteKey string

	// Email
	EmailVerifyLogin bool
	EmailProvider    string
	// SMTP
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPTls      bool

	// Registration
	RegistrationEnabled     bool
	RegistrationVerifyEmail bool

	// SessionBindIP: when true, authenticated sessions must match client_ip stored at login (requires TrustProxy enabled behind CDN).
	SessionBindIP bool

	// TrustProxy: when false, client IP is RemoteAddr only (prevents X-Forwarded-For spoofing).
	TrustProxy bool
}
