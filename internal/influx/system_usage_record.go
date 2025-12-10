package influx

import (
	"context"
	"fmt"
	"mosona-manager/internal/config"
	"mosona-manager/pkg/_type"
	"time"
)

func GetSystemUsage() ([]*_type.ServerUsageRecord, error) {
	query := fmt.Sprintf(`from(bucket: "system_usage")
  |> range(start: 0)
  |> filter(fn: (r) => r._measurement == "system_usage")
  |> sort(columns:["_time"])`)

	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)
	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	var history []*_type.ServerUsageRecord
	statusMap := make(map[time.Time]*_type.ServerUsageRecord)

	for result.Next() {
		record := result.Record()
		timestamp := record.Time()

		if _, exists := statusMap[timestamp]; !exists {
			statusMap[timestamp] = &_type.ServerUsageRecord{Time: timestamp}
			history = append(history, statusMap[timestamp])
		}

		field := record.Field()
		value := record.Value()
		status := statusMap[timestamp]

		switch field {
		case "cpu_usage":
			status.CPUUsage = float64(value.(int64)) / 100
		case "memory":
			status.Memory = float64(value.(int64)) / 100
		}
	}

	if result.Err() != nil {
		return nil, result.Err()
	}

	return history, nil
}
