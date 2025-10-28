package influx

import (
	"context"
	"fmt"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"mosona-manager/_type"
	"mosona-manager/config"
	"strconv"
	"time"
)

func AddServerStatus(serverId int64, status _type.ServerStatusType) error {
	point := influxdb2.NewPoint(
		"server_status",
		map[string]string{
			"server_id": strconv.FormatInt(serverId, 10),
		},
		map[string]interface{}{
			"cpu":           status.CPU,
			"mem_total_mb":  status.MemTotalMB,
			"mem_used_mb":   status.MemUsedMB,
			"disk_total_gb": status.DiskTotalGB,
			"disk_used_gb":  status.DiskUsedGB,
			"rx_kib_s":      status.RxKibS,
			"tx_kib_s":      status.TxKibS,
			"rx_total_mb":   status.RxTotalMB,
			"tx_total_mb":   status.TxTotalMB,
			"timestamp":     time.Now().Unix(),
		},
		time.Now(),
	)
	writeAPI := Client.WriteAPIBlocking(config.Conf.InfluxDBOrg, "server_status_raw")
	return writeAPI.WritePoint(context.Background(), point)
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

		switch field {
		case "cpu":
			status.CPU = value.(float64)
		case "mem_total_mb":
			status.MemTotalMB = value.(float64)
		case "mem_used_mb":
			status.MemUsedMB = value.(float64)
		case "disk_total_gb":
			status.DiskTotalGB = value.(float64)
		case "disk_used_gb":
			status.DiskUsedGB = value.(float64)
		case "rx_kib_s":
			status.RxKibS = value.(float64)
		case "tx_kib_s":
			status.TxKibS = value.(float64)
		case "rx_total_mb":
			status.RxTotalMB = value.(float64)
		case "tx_total_mb":
			status.TxTotalMB = value.(float64)
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
  |> group(columns: ["server_id"])
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
		case "disk_total_gb":
			status.DiskTotalGB = value.(float64)
		case "disk_used_gb":
			status.DiskUsedGB = value.(float64)
		case "rx_kib_s":
			status.RxKibS = value.(float64)
		case "tx_kib_s":
			status.TxKibS = value.(float64)
		case "rx_total_mb":
			status.RxTotalMB = value.(float64)
		case "tx_total_mb":
			status.TxTotalMB = value.(float64)
		}
	}

	if result.Err() != nil {
		return nil, result.Err()
	}

	return statusMap, nil
}
