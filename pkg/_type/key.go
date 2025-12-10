package _type

import "time"

type Key struct {
	ID        int64     `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
