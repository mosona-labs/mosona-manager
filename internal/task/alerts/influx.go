package alerttasks

import (
	"context"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/influx"
	"time"
)

func statusWindow(serverId int64, duration int, end time.Time) ([]*_type.ServerStatusType, error) {
	start := end.Add(-time.Duration(duration) * time.Minute)

	bucket := "server_status_raw"

	query := fmt.Sprintf(`from(bucket: "%s")
		|> range(start: %s, stop: %s)
		|> filter(fn: (r) => r._measurement == "server_status")
		|> filter(fn: (r) => r.server_id == "%d")
		|> sort(columns: ["_time"])`,
		bucket,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		serverId,
	)

	queryAPI := influx.Client.QueryAPI(config.Conf.InfluxDBOrg)
	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = result.Close()
	}()

	statusMap := make(map[time.Time]*_type.ServerStatusType)
	var history []*_type.ServerStatusType

	for result.Next() {
		record := result.Record()
		timestamp := record.Time()

		if _, exists := statusMap[timestamp]; !exists {
			statusMap[timestamp] = &_type.ServerStatusType{Time: timestamp}
			history = append(history, statusMap[timestamp])
		}

		status := statusMap[timestamp]
		field := record.Field()
		value := record.Value()

		switch field {
		case "cpu":
			if v, ok := value.(float64); ok {
				status.CPU = v
			}
		case "mem_total_mb":
			if v, ok := value.(float64); ok {
				status.MemTotalMB = v
			}
		case "mem_used_mb":
			if v, ok := value.(float64); ok {
				status.MemUsedMB = v
			}
		case "swap_total_mb":
			if v, ok := value.(float64); ok {
				status.SwapTotalMB = v
			}
		case "swap_used_mb":
			if v, ok := value.(float64); ok {
				status.SwapUsedMB = v
			}
		case "disks":
			if v, ok := value.(string); ok && v != "" {
				influx.ParseDisksField(status, field, value)
			}
		case "disk_total_gb", "disk_used_gb":
			influx.ParseDisksField(status, field, value)
		case "disk_read_kib_s":
			if v, ok := value.(float64); ok {
				status.DiskReadKibS = v
			}
		case "disk_write_kib_s":
			if v, ok := value.(float64); ok {
				status.DiskWriteKibS = v
			}
		case "disk_read_iops":
			if v, ok := value.(float64); ok {
				status.DiskReadIOPS = v
			}
		case "disk_write_iops":
			if v, ok := value.(float64); ok {
				status.DiskWriteIOPS = v
			}
		case "rx_kib_s":
			if v, ok := value.(float64); ok {
				status.RxKibS = v
			}
		case "tx_kib_s":
			if v, ok := value.(float64); ok {
				status.TxKibS = v
			}
		case "rx_total_mb":
			if v, ok := value.(float64); ok {
				status.RxTotalMB = v
			}
		case "tx_total_mb":
			if v, ok := value.(float64); ok {
				status.TxTotalMB = v
			}
		case "tcp_total":
			if v, ok := value.(int64); ok {
				status.TCPTotal = v
			}
		case "udp_total":
			if v, ok := value.(int64); ok {
				status.UDPTotal = v
			}
		}
	}

	if result.Err() != nil {
		return nil, result.Err()
	}

	return history, nil
}
