package influx

import (
	"context"
	"fmt"
	"mosona-manager/_type"
	"mosona-manager/config"
)

func GetLogsByPage(teamID int64, page, pageSize int, category, level string, userIDs []int64, message string) ([]_type.Log, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize
	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)

	categoryFilter := ""
	if category != "" && category != "all" {
		categoryFilter = fmt.Sprintf(`
			|> filter(fn: (r) => r["category"] == "%s")`, category)
	}
	levelFilter := ""
	if level != "" && level != "all" {
		levelFilter = fmt.Sprintf(`
			|> filter(fn: (r) => r["level"] == "%s")`, level)
	}
	userFilter := ""
	if len(userIDs) > 0 {
		userFilter = "\n|> filter(fn: (r) => ("
		for i, uid := range userIDs {
			if i > 0 {
				userFilter += " or "
			}
			userFilter += fmt.Sprintf(`r["user_id"] == %d`, uid)
		}
		userFilter += "))"
	}
	var messageFilter string
	if message != "" {
		messageFilter = fmt.Sprintf(`
			|> filter(fn: (r) => strings.contains(v: r["message"], substr: ["%s"]))`, message)
	}

	fmt.Println(messageFilter)

	countQuery := fmt.Sprintf(`
		from(bucket: "logs")
			|> range(start: 0)
			|> filter(fn: (r) => r._measurement == "logs")
			|> filter(fn: (r) => r["team_id"] == "%d")
			|> pivot(rowKey:["_time"], columnKey: ["_field"], valueColumn: "_value")%s%s%s%s
			|> group()
			|> count(column: "_measurement")
	`, teamID, categoryFilter, levelFilter, userFilter, messageFilter)
	countResult, err := queryAPI.Query(context.Background(), countQuery)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	for countResult.Next() {
		if count, ok := countResult.Record().Values()["_measurement"]; ok {
			if val, ok := count.(int64); ok {
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
			|> pivot(rowKey:["_time"], columnKey: ["_field"], valueColumn: "_value")%s%s%s%s
			|> sort(columns: ["_time"], desc: true)
			|> limit(n: %d, offset: %d)
	`, teamID, categoryFilter, levelFilter, userFilter, messageFilter, pageSize, offset)

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
