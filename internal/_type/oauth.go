package _type

import "time"

type PublicAuthProvider struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

type AuthProvider struct {
	ID                       int       `db:"id" json:"id"`
	Name                     string    `db:"name" json:"name"`
	Icon                     string    `db:"icon" json:"icon"`
	Protocol                 string    `db:"protocol" json:"protocol"`
	IssuerUrl                string    `db:"issuer_url" json:"issuer_url"`
	AuthUrl                  string    `db:"auth_url" json:"auth_url"`
	TokenUrl                 string    `db:"token_url" json:"token_url"`
	UserinfoUrl              string    `db:"userinfo_url" json:"userinfo_url"`
	Scopes                   string    `db:"scopes" json:"scopes"`
	SubjectField             string    `db:"subject_field" json:"subject_field"`
	IdentityNamespaceVersion int64     `db:"identity_namespace_version" json:"-"`
	ConfigRevision           int64     `db:"config_revision" json:"-"`
	ClientID                 string    `db:"client_id" json:"client_id"`
	ClientSecret             string    `db:"client_secret" json:"client_secret"`
	Skip2FA                  bool      `db:"skip_2fa" json:"skip_2fa"`
	IsEnabled                bool      `db:"is_enabled" json:"is_enabled"`
	Sort                     int       `db:"sort" json:"sort"`
	CreatedAt                time.Time `db:"created_at" json:"created_at"`
	UpdatedAt                time.Time `db:"updated_at" json:"updated_at"`
}

type PublicAuthIdentity struct {
	Id     int    `json:"id"`
	Name   string `json:"name"`
	Icon   string `json:"icon"`
	Linked struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"linked"`
}

type AuthIdentity struct {
	ID          int64  `db:"id" json:"id"`
	UserID      int64  `db:"user_id" json:"user_id"`
	ProviderID  int    `db:"provider_id" json:"provider_id"`
	Subject     string `db:"subject" json:"subject"`
	Email       string `db:"email" json:"email"`
	Name        string `db:"name" json:"name"`
	Quarantined bool   `db:"quarantined" json:"quarantined"`
}
