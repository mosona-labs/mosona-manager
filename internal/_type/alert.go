package _type

import "time"

type ServerAlert struct {
	ID           int64      `json:"id" db:"id"`
	Item         string     `json:"item" db:"item"`
	Threshold    int        `json:"threshold" db:"threshold"`
	ForDuration  int        `json:"for_duration" db:"for_duration"`
	LastStatus   *bool      `json:"last_status,omitempty" db:"last_status"`
	LastNotifyAt *time.Time `json:"last_notify_at,omitempty" db:"last_notify_at"`
}
