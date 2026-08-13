package influx

import (
	"context"
	"encoding/json"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"strconv"
	"strings"
	"time"
)

// ParseDisksField parses the disks JSON string from InfluxDB.
// For backward compatibility, if old disk_total_gb/disk_used_gb fields are present
// instead of the new disks field, they are converted to a single-element Disks slice.
func ParseDisksField(status *_type.ServerStatusType, field string, value interface{}) {
	switch field {
	case "disks":
		if v, ok := value.(string); ok && v != "" {
			var disks []_type.DiskInfo
			if err := json.Unmarshal([]byte(v), &disks); err == nil {
				status.Disks = disks
			}
		}
	case "disk_total_gb":
		// Backward compat: old single-disk format
		if v, ok := value.(float64); ok {
			if len(status.Disks) == 0 {
				status.Disks = []_type.DiskInfo{{MountPoint: "/"}}
			}
			status.Disks[0].TotalGB = v
		}
	case "disk_used_gb":
		if v, ok := value.(float64); ok {
			if len(status.Disks) == 0 {
				status.Disks = []_type.DiskInfo{{MountPoint: "/"}}
			}
			status.Disks[0].UsedGB = v
		}
	}
}

func mapStatusField(status *_type.ServerStatusType, field string, value interface{}) {
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
	case "disks", "disk_total_gb", "disk_used_gb":
		ParseDisksField(status, field, value)
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

func historyFieldFilter(fields []string) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, fmt.Sprintf(`r._field == "%s"`, field))
	}
	return strings.Join(parts, " or ")
}

func buildRaw15sAvgHistoryQuery(serverID int64, start, end time.Time) string {
	numericFields := historyFieldFilter([]string{
		"cpu",
		"mem_total_mb",
		"mem_used_mb",
		"swap_total_mb",
		"swap_used_mb",
		"disk_read_kib_s",
		"disk_write_kib_s",
		"disk_read_iops",
		"disk_write_iops",
		"rx_kib_s",
		"tx_kib_s",
		"rx_total_mb",
		"tx_total_mb",
	})
	stateFields := historyFieldFilter([]string{
		"disks",
		"disk_total_gb",
		"disk_used_gb",
	})

	return fmt.Sprintf(`numeric = from(bucket: "server_status_raw")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status" and r.server_id == "%d" and (%s))
  |> aggregateWindow(every: 15s, fn: mean, createEmpty: false)

state = from(bucket: "server_status_raw")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status" and r.server_id == "%d" and (%s))
  |> aggregateWindow(every: 15s, fn: last, createEmpty: false)

union(tables: [numeric, state])
  |> sort(columns: ["_time"])`,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		serverID,
		numericFields,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		serverID,
		stateFields,
	)
}

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
		mapStatusField(status, field, value)
	}
	status.Time = latestTime

	if result.Err() != nil {
		return nil, result.Err()
	}
	return status, nil
}

func GetLatestServerStatusBatch(serverIDs []int64) (map[int64]*_type.ServerStatusType, error) {
	return GetLatestServerStatusBatchContext(context.Background(), serverIDs)
}

func GetLatestServerStatusBatchContext(ctx context.Context, serverIDs []int64) (map[int64]*_type.ServerStatusType, error) {
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
	result, err := queryAPI.Query(ctx, query)
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

		status := statusMap[serverID]
		status.Time = record.Time()
		mapStatusField(status, record.Field(), record.Value())
	}

	if result.Err() != nil {
		return nil, result.Err()
	}

	return statusMap, nil
}

func GetServerStatusHistory(serverID int64, start, end time.Time, timeFrame string) ([]*_type.ServerStatusType, error) {
	if timeFrame == "raw_15s_avg" {
		queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)
		result, err := queryAPI.Query(context.Background(), buildRaw15sAvgHistoryQuery(serverID, start, end))
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
			mapStatusField(statusMap[timestamp], record.Field(), record.Value())
		}
		if result.Err() != nil {
			return nil, result.Err()
		}
		return history, nil
	}

	var bucket = "server_status_raw"
	switch timeFrame {
	case "minute":
		bucket = "server_status_minute"
	case "hour":
		bucket = "server_status_hourly"
	case "day":
		bucket = "server_status_daily"
	}

	fields := historyFieldFilter([]string{
		"cpu",
		"mem_total_mb",
		"mem_used_mb",
		"swap_total_mb",
		"swap_used_mb",
		"disks",
		"disk_total_gb",
		"disk_used_gb",
		"disk_read_kib_s",
		"disk_write_kib_s",
		"disk_read_iops",
		"disk_write_iops",
		"rx_kib_s",
		"tx_kib_s",
		"rx_total_mb",
		"tx_total_mb",
	})

	query := fmt.Sprintf(`from(bucket: "%s")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status" and r.server_id == "%d" and (%s))
  |> sort(columns:["_time"])`, bucket, start.Format(time.RFC3339), end.Format(time.RFC3339), serverID, fields)

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

		mapStatusField(statusMap[timestamp], record.Field(), record.Value())
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
