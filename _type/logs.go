package _type

import "time"

type Log struct {
	UserID        int64     `json:"user_id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	Category      string    `json:"category"`
	Message       string    `json:"message"`
	IP            string    `json:"ip"`
	IPCountry     string    `json:"ip_country"`
	IPCountryCode string    `json:"ip_country_code"`
	UserAgent     string    `json:"user_agent"`
	Level         string    `json:"level"`
	Time          time.Time `json:"time"`
}
