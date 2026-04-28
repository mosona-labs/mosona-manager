package _type

import "time"

type PublicMonitor struct {
	ID          int64      `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	Category    int64      `json:"category" db:"category"`
	Weight      int        `json:"weight" db:"weight"`
	OS          *string    `json:"os" db:"os"`
	County      *string    `json:"county" db:"county"`
	Area        *string    `json:"area" db:"area"`
	CoreC       *int       `json:"core_c" db:"core_c"`
	CoreT       *int       `json:"core_t" db:"core_t"`
	OpenTime    *time.Time `json:"open_time" db:"open_time"`
	Provider    *string    `json:"provider" db:"provider"`
	Cycle       *int16     `json:"cycle" db:"cycle"`
	StartTime   *time.Time `json:"start_time" db:"start_time"`
	EndTime     *time.Time `json:"end_time" db:"end_time"`
	Amount      *string    `json:"amount" db:"amount"`
	Bandwidth   *string    `json:"bandwidth" db:"bandwidth"`
	Traffic     *string    `json:"traffic" db:"traffic"`
	TrafficType *int16     `json:"traffic_type" db:"traffic_type"`
	NotePublic  *string    `json:"note_public" db:"note_public"`
}
