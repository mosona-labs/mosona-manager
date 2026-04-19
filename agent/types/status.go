package types

type DiskInfo struct {
	MountPoint string  `msgpack:"mp"`
	TotalGB    float64 `msgpack:"total_gb"`
	UsedGB     float64 `msgpack:"used_gb"`
}

type Status struct {
	CPU           float64    `msgpack:"cpu"`
	MemTotalMB    float64    `msgpack:"mem_total_mb"`
	MemUsedMB     float64    `msgpack:"mem_used_mb"`
	SwapTotalMB   float64    `msgpack:"swap_total_mb"`
	SwapUsedMB    float64    `msgpack:"swap_used_mb"`
	Disks         []DiskInfo `msgpack:"disks"`
	DiskReadKibS  float64    `msgpack:"disk_read_kib_s"`
	DiskWriteKibS float64    `msgpack:"disk_write_kib_s"`
	DiskReadIOPS  float64    `msgpack:"disk_read_iops"`
	DiskWriteIOPS float64    `msgpack:"disk_write_iops"`
	RxKibS        float64    `msgpack:"rx_kib_s"`
	TxKibS        float64    `msgpack:"tx_kib_s"`
	RxTotalMB     float64    `msgpack:"rx_total_mb"`
	TxTotalMB     float64    `msgpack:"tx_total_mb"`
	TCPTotal      int64      `msgpack:"tcp_total"`
	UDPTotal      int64      `msgpack:"udp_total"`
}
