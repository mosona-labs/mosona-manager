package msettings

type Response struct {
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

	// Registration
	RegistrationEnabled     bool `json:"registration_enabled"`
	RegistrationVerifyEmail bool `json:"registration_verify_email"`

	// Captcha
	CaptchaSiteKey   string `json:"captcha_site_key"`
	CaptchaSecretKey string `json:"captcha_secret_key"`
}
