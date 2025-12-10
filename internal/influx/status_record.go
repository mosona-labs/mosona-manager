package influx

import (
	"context"
	"fmt"
	"mosona-manager/internal/config"
	"mosona-manager/pkg/_type"
	"strconv"
	"time"
)

func GetLatestServerStatus(serverID int64) (*_type.ServerStatusType, error) {
	query := fmt.Sprintf(`from(bucket: "%s")
  |> range(start: -30d)
  |> filter(fn: (r) => r._measurement == "server_status" and r.server_id == "%d")
  |> last()`, "server_status_raw", serverID)

	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)

	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	status := &_type.ServerStatusType{}
	var latestTime time.Time

	for result.Next() {
		field := result.Record().Field()
		value := result.Record().Value()
		latestTime = result.Record().Time()

		switch field {
		case "cpu":
			status.CPU = value.(float64)
		case "mem_total_mb":
			status.MemTotalMB = value.(float64)
		case "mem_used_mb":
			status.MemUsedMB = value.(float64)
		case "swap_total_mb":
			status.SwapTotalMB = value.(float64)
		case "swap_used_mb":
			status.SwapUsedMB = value.(float64)
		case "disk_total_gb":
			status.DiskTotalGB = value.(float64)
		case "disk_used_gb":
			status.DiskUsedGB = value.(float64)
		case "disk_read_kib_s":
			status.DiskReadKibS = value.(float64)
		case "disk_write_kib_s":
			status.DiskWriteKibS = value.(float64)
		case "disk_read_iops":
			status.DiskReadIOPS = value.(float64)
		case "disk_write_iops":
			status.DiskWriteIOPS = value.(float64)
		case "rx_kib_s":
			status.RxKibS = value.(float64)
		case "tx_kib_s":
			status.TxKibS = value.(float64)
		case "rx_total_mb":
			status.RxTotalMB = value.(float64)
		case "tx_total_mb":
			status.TxTotalMB = value.(float64)
		case "tcp_total":
			status.TCPTotal = value.(int64)
		case "udp_total":
			status.UDPTotal = value.(int64)
		}
	}
	status.Time = latestTime

	if result.Err() != nil {
		return nil, result.Err()
	}
	return status, nil
}

func GetLatestServerStatusBatch(serverIDs []int64) (map[int64]*_type.ServerStatusType, error) {
	if len(serverIDs) == 0 {
		return make(map[int64]*_type.ServerStatusType), nil
	}

	idFilter := ""
	for i, id := range serverIDs {
		if i > 0 {
			idFilter += " or "
		}
		idFilter += fmt.Sprintf(`r.server_id == "%d"`, id)
	}

	query := fmt.Sprintf(`from(bucket: "%s")
  |> range(start: -30d)
  |> filter(fn: (r) => r._measurement == "server_status" and (%s))
  |> last()`, "server_status_raw", idFilter)

	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)
	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	statusMap := make(map[int64]*_type.ServerStatusType)

	for result.Next() {
		record := result.Record()
		serverIDStr, ok := record.ValueByKey("server_id").(string)
		if !ok {
			continue
		}
		serverID, err := strconv.ParseInt(serverIDStr, 10, 64)
		if err != nil {
			continue
		}

		if _, exists := statusMap[serverID]; !exists {
			statusMap[serverID] = &_type.ServerStatusType{}
		}

		field := record.Field()
		value := record.Value()
		status := statusMap[serverID]
		status.Time = record.Time()

		switch field {
		case "cpu":
			status.CPU = value.(float64)
		case "mem_total_mb":
			status.MemTotalMB = value.(float64)
		case "mem_used_mb":
			status.MemUsedMB = value.(float64)
		case "swap_total_mb":
			status.SwapTotalMB = value.(float64)
		case "swap_used_mb":
			status.SwapUsedMB = value.(float64)
		case "disk_total_gb":
			status.DiskTotalGB = value.(float64)
		case "disk_used_gb":
			status.DiskUsedGB = value.(float64)
		case "disk_read_kib_s":
			status.DiskReadKibS = value.(float64)
		case "disk_write_kib_s":
			status.DiskWriteKibS = value.(float64)
		case "disk_read_iops":
			status.DiskReadIOPS = value.(float64)
		case "disk_write_iops":
			status.DiskWriteIOPS = value.(float64)
		case "rx_kib_s":
			status.RxKibS = value.(float64)
		case "tx_kib_s":
			status.TxKibS = value.(float64)
		case "rx_total_mb":
			status.RxTotalMB = value.(float64)
		case "tx_total_mb":
			status.TxTotalMB = value.(float64)
		case "tcp_total":
			status.TCPTotal = value.(int64)
		case "udp_total":
			status.UDPTotal = value.(int64)
		}
	}

	if result.Err() != nil {
		return nil, result.Err()
	}

	return statusMap, nil
}

func GetServerStatusHistory(serverID int64, start, end time.Time, timeFrame string) ([]*_type.ServerStatusType, error) {
	var bucket = "server_status_raw"
	switch timeFrame {
	case "minute":
		bucket = "server_status_minute"
	case "hour":
		bucket = "server_status_hourly"
	case "day":
		bucket = "server_status_daily"
	}

	query := fmt.Sprintf(`from(bucket: "%s")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status" and r.server_id == "%d")
  |> sort(columns:["_time"])`, bucket, start.Format(time.RFC3339), end.Format(time.RFC3339), serverID)

	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)
	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	var history []*_type.ServerStatusType
	statusMap := make(map[time.Time]*_type.ServerStatusType)

	for result.Next() {
		record := result.Record()
		timestamp := record.Time()

		if _, exists := statusMap[timestamp]; !exists {
			statusMap[timestamp] = &_type.ServerStatusType{Time: timestamp}
			history = append(history, statusMap[timestamp])
		}

		field := record.Field()
		value := record.Value()
		status := statusMap[timestamp]

		switch field {
		case "cpu":
			status.CPU = value.(float64)
		case "mem_total_mb":
			status.MemTotalMB = value.(float64)
		case "mem_used_mb":
			status.MemUsedMB = value.(float64)
		case "swap_total_mb":
			status.SwapTotalMB = value.(float64)
		case "swap_used_mb":
			status.SwapUsedMB = value.(float64)
		case "disk_total_gb":
			status.DiskTotalGB = value.(float64)
		case "disk_used_gb":
			status.DiskUsedGB = value.(float64)
		case "disk_read_kib_s":
			status.DiskReadKibS = value.(float64)
		case "disk_write_kib_s":
			status.DiskWriteKibS = value.(float64)
		case "disk_read_iops":
			status.DiskReadIOPS = value.(float64)
		case "disk_write_iops":
			status.DiskWriteIOPS = value.(float64)
		case "rx_kib_s":
			status.RxKibS = value.(float64)
		case "tx_kib_s":
			status.TxKibS = value.(float64)
		case "rx_total_mb":
			status.RxTotalMB = value.(float64)
		case "tx_total_mb":
			status.TxTotalMB = value.(float64)
		case "tcp_total":
			status.TCPTotal = value.(int64)
		case "udp_total":
			status.UDPTotal = value.(int64)
		}
	}

	if result.Err() != nil {
		return nil, result.Err()
	}

	return history, nil
}

func GetAllServerRecordCount(bucket string) (int64, error) {
	query := `from(bucket: "` + bucket + `")
  |> range(start: -365d)
  |> filter(fn: (r) => r._measurement == "server_status")
  |> count()`

	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)
	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return 0, err
	}

	var totalCount int64 = 0

	for result.Next() {
		if count, ok := result.Record().Value().(int64); ok {
			totalCount += count
		}
	}

	if result.Err() != nil {
		return 0, result.Err()
	}

	return totalCount, nil
}

func GetAllBucketAllServerRecordCount() (int64, error) {
	var count int64 = 0
	buckets := []string{"server_status_raw", "server_status_minute", "server_status_hourly", "server_status_daily"}
	for _, bucket := range buckets {
		bucketCount, err := GetAllServerRecordCount(bucket)
		if err != nil {
			return 0, err
		}
		count += bucketCount
	}
	return count, nil
}
