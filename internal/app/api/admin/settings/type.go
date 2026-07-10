package msettings

type Response struct {
	Debug bool `json:"debug"`

	// Site
	Title   string `json:"title"`
	Favicon string `json:"favicon"`

	// Domain
	Domain string `json:"domain"`

	// Email
	EmailProvider string `json:"email_provider"`
	// SMTP
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPTls      bool   `json:"smtp_tls"`

	// Login
	EmailVerifyLogin bool `json:"email_verify_login"`
	SessionBindIP    bool `json:"session_bind_ip"`
	TrustProxy       bool `json:"trust_proxy"`

	// Registration
	RegistrationEnabled     bool `json:"registration_enabled"`
	RegistrationVerifyEmail bool `json:"registration_verify_email"`

	// Captcha
	CaptchaSiteKey   string `json:"captcha_site_key"`
	CaptchaSecretKey string `json:"captcha_secret_key"`
}
