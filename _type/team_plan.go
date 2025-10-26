package _type

import "time"

type TeamPlan struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       int64     `json:"price"`
	MaxServer   int       `json:"max_server" db:"max_server"`
	MaxAlert    int       `json:"max_alert" db:"max_alert"`
	MaxMember   int       `json:"max_member" db:"max_member"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}
