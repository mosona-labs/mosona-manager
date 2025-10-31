package _type

import "time"

type ServerInfoAdv struct {
	Hostname *string `json:"hostname"`
	CPUName  *string `json:"cpu_name" db:"cpu_name"`
	CoreC    *int    `json:"core_c" db:"core_c"`
	CoreT    *int    `json:"core_t" db:"core_t"`
	Kernel   *string `json:"kernel"`
	IP       *string `json:"ip"`
	Arch     *string `json:"arch"`
}

type Monitor struct {
	ServerMinimal
	OS          *string    `json:"os"`
	County      *string    `json:"county"`
	Area        *string    `json:"area"`
	OpenTime    *time.Time `json:"open_time" db:"open_time"`
	Note        *string    `json:"note"`
	Provider    *string    `json:"provider"`
	Cycle       *int8      `json:"cycle"`
	StartTime   *time.Time `json:"start_time" db:"start_time"`
	EndTime     *time.Time `json:"end_time" db:"end_time"`
	Amount      *string    `json:"amount"`
	Bandwidth   *string    `json:"bandwidth"`
	Traffic     *string    `json:"traffic"`
	TrafficType *int8      `json:"traffic_type" db:"traffic_type"`
	NotePublic  *string    `json:"note_public" db:"note_public"`
}

type MonitorDetail struct {
	Monitor
	ServerInfoAdv
}
