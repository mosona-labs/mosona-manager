package influx

import (
	"context"
	"fmt"
	"mosona-manager/_type"
	"mosona-manager/config"
)

func GetLogsByPage(teamID int64, page, pageSize int) ([]_type.Log, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize
	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)

	countQuery := fmt.Sprintf(`
		from(bucket: "logs")
			|> range(start: 0)
			|> filter(fn: (r) => r._measurement == "logs")
			|> filter(fn: (r) => r["team_id"] == "%d")
			|> count()
	`, teamID)
	countResult, err := queryAPI.Query(context.Background(), countQuery)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	for countResult.Next() {
		if countResult.Record().Field() == "user_id" {
			if val, ok := countResult.Record().Value().(int64); ok {
				total = val
				break
			}
		}
	}
	_ = countResult.Close()

	dataQuery := fmt.Sprintf(`
		from(bucket: "logs")
			|> range(start: 0)
			|> filter(fn: (r) => r._measurement == "logs")
			|> filter(fn: (r) => r["team_id"] == "%d")
			|> pivot(rowKey:["_time"], columnKey: ["_field"], valueColumn: "_value")
			|> sort(columns: ["_time"], desc: true)
			|> limit(n: %d, offset: %d)
	`, teamID, pageSize, offset)

	result, err := queryAPI.Query(context.Background(), dataQuery)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		_ = result.Close()
	}()

	logs := make([]_type.Log, 0)
	for result.Next() {
		record := result.Record()
		log := _type.Log{
			Time: record.Time(),
		}

		if val, ok := record.ValueByKey("user_id").(int64); ok {
			log.UserID = val
		}
		if val, ok := record.ValueByKey("category").(string); ok {
			log.Category = val
		}
		if val, ok := record.ValueByKey("message").(string); ok {
			log.Message = val
		}
		if val, ok := record.ValueByKey("ip").(string); ok {
			log.IP = val
		}
		if val, ok := record.ValueByKey("ip_country").(string); ok {
			log.IPCountry = val
		}
		if val, ok := record.ValueByKey("ip_country_code").(string); ok {
			log.IPCountryCode = val
		}
		if val, ok := record.ValueByKey("user_agent").(string); ok {
			log.UserAgent = val
		}
		if val, ok := record.ValueByKey("level").(string); ok {
			log.Level = val
		}

		logs = append(logs, log)
	}

	if result.Err() != nil {
		return nil, 0, result.Err()
	}

	return logs, total, nil
}
