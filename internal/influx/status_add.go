package influx

import (
	"context"
	"mosona-manager/internal/config"
	"mosona-manager/pkg/_type"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

func AddServerStatus(serverId int64, status _type.ServerStatusType) error {
	point := influxdb2.NewPoint(
		"server_status",
		map[string]string{
			"server_id": strconv.FormatInt(serverId, 10),
		},
		map[string]interface{}{
			"cpu":              status.CPU,
			"mem_total_mb":     status.MemTotalMB,
			"mem_used_mb":      status.MemUsedMB,
			"swap_total_mb":    status.SwapTotalMB,
			"swap_used_mb":     status.SwapUsedMB,
			"disk_total_gb":    status.DiskTotalGB,
			"disk_used_gb":     status.DiskUsedGB,
			"disk_read_kib_s":  status.DiskReadKibS,
			"disk_write_kib_s": status.DiskWriteKibS,
			"disk_read_iops":   status.DiskReadIOPS,
			"disk_write_iops":  status.DiskWriteIOPS,
			"rx_kib_s":         status.RxKibS,
			"tx_kib_s":         status.TxKibS,
			"rx_total_mb":      status.RxTotalMB,
			"tx_total_mb":      status.TxTotalMB,
			"tcp_total":        status.TCPTotal,
			"udp_total":        status.UDPTotal,
		},
		time.Now(),
	)
	writeAPI := Client.WriteAPIBlocking(config.Conf.InfluxDBOrg, "server_status_raw")
	return writeAPI.WritePoint(context.Background(), point)
}
