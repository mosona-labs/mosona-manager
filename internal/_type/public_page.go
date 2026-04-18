package _type

import "time"

type TeamPublicPage struct {
	TeamID      int64     `json:"team_id,omitempty" db:"team_id"`
	Enabled     bool      `json:"enabled" db:"enabled"`
	Name        *string   `json:"name,omitempty" db:"name"`
	Domain      *string   `json:"domain,omitempty" db:"domain"`
	Title       *string   `json:"title,omitempty" db:"title"`
	Description *string   `json:"description,omitempty" db:"description"`
	CreatedAt   time.Time `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at,omitempty" db:"updated_at"`
}

type ResolvedPublicPage struct {
	TeamPublicPage
	TeamName string `json:"team_name,omitempty" db:"team_name"`
}

type PublicPageSummary struct {
	Title       string  `json:"title"`
	Name        *string `json:"name,omitempty"`
	Domain      *string `json:"domain,omitempty"`
	Description *string `json:"description,omitempty"`
}
