package _type

type PublicAuthProvider struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

type AuthProvider struct {
	ID           int    `db:"id" json:"id"`
	Name         string `db:"name" json:"name"`
	Icon         string `db:"icon" json:"icon"`
	AuthUrl      string `db:"auth_url" json:"auth_url"`
	TokenUrl     string `db:"token_url" json:"token_url"`
	UserinfoUrl  string `db:"userinfo_url" json:"userinfo_url"`
	ClientID     string `db:"client_id" json:"client_id"`
	ClientSecret string `db:"client_secret" json:"client_secret"`
	Skip2FA      bool   `db:"skip_2fa" json:"skip_2fa"`
	IsEnabled    bool   `db:"is_enabled" json:"is_enabled"`
	CreatedAt    string `db:"created_at" json:"created_at"`
	UpdatedAt    string `db:"updated_at" json:"updated_at"`
}

type AuthIdentity struct {
	ID          int64  `db:"id" json:"id"`
	UserID      int64  `db:"user_id" json:"user_id"`
	ProviderID  int    `db:"provider_id" json:"provider_id"`
	Subject     string `db:"subject" json:"subject"`
	Email       string `db:"email" json:"email"`
	Name        string `db:"name" json:"name"`
	LastLoginAt string `db:"last_login_at" json:"last_login_at"`
}
