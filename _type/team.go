package _type

import "time"

type Team struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Color       string    `json:"color"`
	Image       string    `json:"image"`
	MaxServer   int       `json:"max_server" db:"max_server"`
	MaxAlert    int       `json:"max_alert" db:"max_alert"`
	MaxMember   int       `json:"max_member" db:"max_member"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

type TeamMemberRole struct {
	UID  int64 `json:"id"`
	Role int   `json:"role"`
}

type TeamUsersRole struct {
	User
	Role int `json:"role"`
}
