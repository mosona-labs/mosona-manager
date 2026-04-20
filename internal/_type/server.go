package _type

import (
	"time"
)

type ServerMinimal struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     int16  `json:"type"` // 0: SSH, 1: Agent (a), 2: Agent (p)
	Weight   int    `json:"weight"`
	Category int64  `json:"category"`
}

type DiskInfo struct {
	MountPoint string  `json:"mp"`
	TotalGB    float64 `json:"total_gb"`
	UsedGB     float64 `json:"used_gb"`
}

type ServerStatusType struct {
	CPU           float64    `json:"cpu"`
	MemTotalMB    float64    `json:"mem_total_mb"`
	MemUsedMB     float64    `json:"mem_used_mb"`
	SwapTotalMB   float64    `json:"swap_total_mb"`
	SwapUsedMB    float64    `json:"swap_used_mb"`
	Disks         []DiskInfo `json:"disks"`
	DiskReadKibS  float64    `json:"disk_read_kib_s"`
	DiskWriteKibS float64    `json:"disk_write_kib_s"`
	DiskReadIOPS  float64    `json:"disk_read_iops"`
	DiskWriteIOPS float64    `json:"disk_write_iops"`
	RxKibS        float64    `json:"rx_kib_s"`
	TxKibS        float64    `json:"tx_kib_s"`
	RxTotalMB     float64    `json:"rx_total_mb"`
	TxTotalMB     float64    `json:"tx_total_mb"`
	TCPTotal      int64      `json:"tcp_total"`
	UDPTotal      int64      `json:"udp_total"`
	Time          time.Time  `json:"time"`
}

type ServerChartStatusType struct {
	CPU           float64    `json:"cpu"`
	MemTotalMB    float64    `json:"mem_total_mb"`
	MemUsedMB     float64    `json:"mem_used_mb"`
	SwapTotalMB   float64    `json:"swap_total_mb"`
	SwapUsedMB    float64    `json:"swap_used_mb"`
	Disks         []DiskInfo `json:"disks"`
	DiskReadKibS  float64    `json:"disk_read_kib_s"`
	DiskWriteKibS float64    `json:"disk_write_kib_s"`
	DiskReadIOPS  float64    `json:"disk_read_iops"`
	DiskWriteIOPS float64    `json:"disk_write_iops"`
	RxKibS        float64    `json:"rx_kib_s"`
	TxKibS        float64    `json:"tx_kib_s"`
	RxTotalMB     float64    `json:"rx_total_mb"`
	TxTotalMB     float64    `json:"tx_total_mb"`
	Time          time.Time  `json:"time"`
}

type ServerInfoType struct {
	SystemVersion string `json:"system_version"`
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
	Type          int16      `json:"type" db:"type"`
	Name          string     `json:"name" db:"name"`
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

	// Connection
	Address string `json:"address" db:"address"`
	Port    int    `json:"port" db:"port"`

	// SSH
	Username string `json:"username" db:"username"`
	Password string `json:"password" db:"password"`
	KeyID    int64  `json:"key_id" db:"key_id"`

	// Agent
	AgentStatus     int        `json:"agent_status"` // 0: Not installed, 1: Installed
	AgentVersion    *string    `json:"agent_version"`
	AgentLastSeenAt *time.Time `json:"agent_last_seen_at"`

	// Active
	AgentUUID *string `json:"agent_uuid"`
}
