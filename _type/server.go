package _type

import "time"

type Server struct {
	ID            int64     `json:"id"`
	TeamID        int64     `json:"team_id" db:"team_id"`
	Name          string    `json:"name"`
	Address       string    `json:"address"`
	Port          string    `json:"port"`
	AllowMonitor  bool      `json:"allow_monitor" db:"allow_monitor"`
	AllowTerminal bool      `json:"allow_terminal" db:"allow_terminal"`
	Weight        int       `json:"weight"`
	Category      int64     `json:"category"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type ServerConnect struct {
	ID       int64  `json:"id"`
	TeamID   int64  `json:"team_id" db:"team_id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Port     string `json:"port"`
	Key      string `json:"key"`
	Password string `json:"password"`
}

type ServerStatusType struct {
	CPU         float64   `json:"cpu"`
	MemTotalMB  float64   `json:"mem_total_mb"`
	MemUsedMB   float64   `json:"mem_used_mb"`
	DiskTotalGB float64   `json:"disk_total_gb"`
	DiskUsedGB  float64   `json:"disk_used_gb"`
	RxKibS      float64   `json:"rx_kib_s"`
	TxKibS      float64   `json:"tx_kib_s"`
	RxTotalMB   float64   `json:"rx_total_mb"`
	TxTotalMB   float64   `json:"tx_total_mb"`
	Time        time.Time `json:"time"`
}
