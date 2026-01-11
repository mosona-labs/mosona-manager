package _type

import "time"

type User struct {
	ID        int64      `json:"id"`
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	TOTP      *bool      `json:"totp_enabled,omitempty" db:"totp_enabled"`
	IsAdmin   bool       `json:"is_admin" db:"is_admin"`
	Verified  *bool      `json:"verified,omitempty" db:"verified"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	LoginAt   *time.Time `json:"login_at,omitempty" db:"login_at"`
	PwdAt     *time.Time `json:"pwd_at,omitempty" db:"pwd_at"`
}

type UserAuthInfo struct {
	ID       int64   `json:"id"`
	Email    string  `json:"email"`
	Password string  `json:"password"`
	Salt     string  `json:"salt"`
	TOTP     *string `json:"totp"`
	IsAdmin  bool    `db:"is_admin" json:"is_admin"`
	Verified bool    `json:"verified"`
}
