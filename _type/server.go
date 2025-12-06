package _type

import (
	"time"
)

type Server struct {
	ID            int64     `json:"id"`
	TeamID        int64     `json:"team_id" db:"team_id"`
	Name          string    `json:"name"`
	Address       string    `json:"address"`
	Port          int       `json:"port"`
	AllowMonitor  bool      `json:"allow_monitor" db:"allow_monitor"`
	AllowTerminal bool      `json:"allow_terminal" db:"allow_terminal"`
	Weight        int       `json:"weight"`
	Category      int64     `json:"category"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type ServerMinimal struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Weight   int    `json:"weight"`
	Category int64  `json:"category"`
}

type ServerConnect struct {
	ID       int64  `json:"id"`
	TeamID   int64  `json:"team_id" db:"team_id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Key      string `json:"key"`
	Password string `json:"password"`
}

type ServerStatusType struct {
	CPU           float64   `json:"cpu"`
	MemTotalMB    float64   `json:"mem_total_mb"`
	MemUsedMB     float64   `json:"mem_used_mb"`
	SwapTotalMB   float64   `json:"swap_total_mb"`
	SwapUsedMB    float64   `json:"swap_used_mb"`
	DiskTotalGB   float64   `json:"disk_total_gb"`
	DiskUsedGB    float64   `json:"disk_used_gb"`
	DiskReadKibS  float64   `json:"disk_read_kib_s"`
	DiskWriteKibS float64   `json:"disk_write_kib_s"`
	DiskReadIOPS  float64   `json:"disk_read_iops"`
	DiskWriteIOPS float64   `json:"disk_write_iops"`
	RxKibS        float64   `json:"rx_kib_s"`
	TxKibS        float64   `json:"tx_kib_s"`
	RxTotalMB     float64   `json:"rx_total_mb"`
	TxTotalMB     float64   `json:"tx_total_mb"`
	TCPTotal      int64     `json:"tcp_total"`
	UDPTotal      int64     `json:"udp_total"`
	Time          time.Time `json:"time"`
}

type ServerInfoType struct {
	LinuxVersion  string `json:"linux_version"`
	Uptime        string `json:"uptime"`
	Hostname      string `json:"hostname"`
	CpuName       string `json:"cpu_name"`
	CpuC          int    `json:"cpu_c"`
	CpuT          int    `json:"cpu_t"`
	KernelVersion string `json:"kernel_version"`
	IPAddress     string `json:"ip_address"`
	Architecture  string `json:"architecture"`
}

type ServerFullType struct {
	ID            int64      `json:"id" db:"id"`
	Category      int64      `json:"category" db:"category"`
	Name          string     `json:"name" db:"name"`
	Address       string     `json:"address" db:"address"`
	Port          int        `json:"port" db:"port"`
	Username      string     `json:"username" db:"username"`
	Password      string     `json:"password" db:"password"`
	KeyID         int64      `json:"key_id" db:"key_id"`
	AllowMonitor  bool       `json:"allow_monitor" db:"allow_monitor"`
	AllowTerminal bool       `json:"allow_terminal" db:"allow_terminal"`
	Weight        int        `json:"weight" db:"weight"`
	Note          *string    `json:"note" db:"note"`
	Provider      *string    `json:"provider" db:"provider"`
	Cycle         *int       `json:"cycle" db:"cycle"`
	StartTime     *time.Time `json:"start_time" db:"start_time"`
	EndTime       *time.Time `json:"end_time" db:"end_time"`
	Amount        *string    `json:"amount" db:"amount"`
	AutoRenew     *bool      `json:"auto_renew" db:"auto_renew"`
	Bandwidth     *string    `json:"bandwidth" db:"bandwidth"`
	Traffic       *string    `json:"traffic" db:"traffic"`
	TrafficType   *int       `json:"traffic_type" db:"traffic_type"`
	NotePublic    *string    `json:"note_public" db:"note_public"`
}
