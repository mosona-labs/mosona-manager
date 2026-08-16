package influx

import (
	"context"
	"errors"
	"fmt"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"strconv"
	"strings"
	"time"
)

const (
	maxLogMessageFilterLength = 256
	maxLogPage                = 100_000
	maxLogPageSize            = 1_000
	logQueryTimeout           = 15 * time.Second
)

var ErrInvalidLogFilter = errors.New("invalid log filter")

var validLogCategories = map[string]struct{}{
	"category": {},
	"login":    {},
	"oauth":    {},
	"security": {},
	"server":   {},
	"settings": {},
	"team":     {},
	"terminal": {},
	"user":     {},
}

var validLogLevels = map[string]struct{}{
	"low":    {},
	"medium": {},
	"high":   {},
}

func ValidateLogFilters(category, level, message string) error {
	if category != "" && category != "all" {
		if _, ok := validLogCategories[category]; !ok {
			return fmt.Errorf("%w: category", ErrInvalidLogFilter)
		}
	}
	if level != "" && level != "all" {
		if _, ok := validLogLevels[level]; !ok {
			return fmt.Errorf("%w: level", ErrInvalidLogFilter)
		}
	}
	if len([]rune(message)) > maxLogMessageFilterLength {
		return fmt.Errorf("%w: message is longer than %d characters", ErrInvalidLogFilter, maxLogMessageFilterLength)
	}
	return nil
}

func ValidateLogPagination(page, pageSize int) error {
	if page > maxLogPage {
		return fmt.Errorf("%w: page exceeds %d", ErrInvalidLogFilter, maxLogPage)
	}
	if pageSize > maxLogPageSize {
		return fmt.Errorf("%w: page size exceeds %d", ErrInvalidLogFilter, maxLogPageSize)
	}
	return nil
}

func GetLogsByPage(ctx context.Context, teamID int64, page, pageSize int, category, level string, userIDs []int64, message string) ([]_type.Log, int64, error) {
	if err := ValidateLogFilters(category, level, message); err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if err := ValidateLogPagination(page, pageSize); err != nil {
		return nil, 0, err
	}

	ctx, cancel := context.WithTimeout(ctx, logQueryTimeout)
	defer cancel()

	countQuery, dataQuery := buildLogQueries(teamID, page, pageSize, category, level, userIDs, message)
	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)
	countResult, err := queryAPI.Query(ctx, countQuery)
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
	if err = countResult.Err(); err != nil {
		_ = countResult.Close()
		return nil, 0, err
	}
	_ = countResult.Close()

	result, err := queryAPI.Query(ctx, dataQuery)
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

func buildLogQueries(teamID int64, page, pageSize int, category, level string, userIDs []int64, message string) (string, string) {
	var imports string
	var filters strings.Builder
	if category != "" && category != "all" {
		fmt.Fprintf(&filters, `
		|> filter(fn: (r) => r["category"] == %s)`, fluxStringLiteral(category))
	}
	if level != "" && level != "all" {
		fmt.Fprintf(&filters, `
		|> filter(fn: (r) => r["level"] == %s)`, fluxStringLiteral(level))
	}
	if len(userIDs) > 0 {
		filters.WriteString(`
		|> filter(fn: (r) => (`)
		for i, uid := range userIDs {
			if i > 0 {
				filters.WriteString(" or ")
			}
			fmt.Fprintf(&filters, `r["user_id"] == %d`, uid)
		}
		filters.WriteString("))")
	}
	if message != "" {
		imports = "import \"strings\"\n"
		fmt.Fprintf(&filters, `
		|> filter(fn: (r) => strings.containsStr(v: r["message"], substr: %s))`, fluxStringLiteral(message))
	}

	// Server-side query parameters (params.*) are only supported by InfluxDB
	// Cloud; OSS returns "undefined identifier params", so values are inlined
	// as escaped Flux literals instead.
	baseQuery := fmt.Sprintf(`%sfrom(bucket: "logs")
	|> range(start: -365d)
	|> filter(fn: (r) => r._measurement == "logs")
	|> filter(fn: (r) => r["team_id"] == %s)
	|> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")%s`,
		imports, fluxStringLiteral(strconv.FormatInt(teamID, 10)), filters.String())

	countQuery := baseQuery + `
	|> group()
	|> count(column: "_measurement")`
	dataQuery := fmt.Sprintf(`%s
	|> sort(columns: ["_time"], desc: true)
	|> limit(n: %d, offset: %d)`, baseQuery, pageSize, (page-1)*pageSize)
	return countQuery, dataQuery
}

// fluxStringLiteral renders s as a Flux double-quoted string literal. In
// addition to the usual escapes, "$" is escaped because Flux evaluates
// ${...} interpolation inside string literals.
func fluxStringLiteral(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '$':
			b.WriteString(`\$`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
