package influx

import (
	"context"
	"encoding/base64"
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
	maxLogPageSize            = 1_000
	logQueryTimeout           = 15 * time.Second
	defaultLogRange           = 30 * 24 * time.Hour
	maxLogRange               = 365 * 24 * time.Hour
	maxLogMessageSearchRange  = 30 * 24 * time.Hour
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

type LogPage struct {
	Logs       []_type.Log
	NextCursor string
	HasMore    bool
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

func ValidateLogPageSize(pageSize int) error {
	if pageSize < 1 || pageSize > maxLogPageSize {
		return fmt.Errorf("%w: page size must be between 1 and %d", ErrInvalidLogFilter, maxLogPageSize)
	}
	return nil
}

func ParseLogTimeRange(startValue, endValue, message string, now time.Time) (time.Time, time.Time, error) {
	end := now.UTC()
	if endValue != "" {
		parsed, err := time.Parse(time.RFC3339, endValue)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: invalid end time", ErrInvalidLogFilter)
		}
		end = parsed.UTC()
	}

	start := end.Add(-defaultLogRange)
	if startValue != "" {
		parsed, err := time.Parse(time.RFC3339, startValue)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: invalid start time", ErrInvalidLogFilter)
		}
		start = parsed.UTC()
	}

	duration := end.Sub(start)
	if duration <= 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: start time must be before end time", ErrInvalidLogFilter)
	}
	if duration > maxLogRange {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: time range exceeds %s", ErrInvalidLogFilter, maxLogRange)
	}
	if message != "" && duration > maxLogMessageSearchRange {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: message search range exceeds %s", ErrInvalidLogFilter, maxLogMessageSearchRange)
	}
	return start, end, nil
}

func GetLogs(ctx context.Context, teamID int64, pageSize int, category, level string, userIDs []int64, message string, start, end time.Time, cursor string) (LogPage, error) {
	if err := ValidateLogFilters(category, level, message); err != nil {
		return LogPage{}, err
	}
	if err := ValidateLogPageSize(pageSize); err != nil {
		return LogPage{}, err
	}
	if !start.Before(end) || end.Sub(start) > maxLogRange || (message != "" && end.Sub(start) > maxLogMessageSearchRange) {
		return LogPage{}, fmt.Errorf("%w: invalid time range", ErrInvalidLogFilter)
	}

	queryEnd := end.UTC()
	if cursor != "" {
		cursorTime, err := decodeLogCursor(cursor)
		if err != nil {
			return LogPage{}, err
		}
		if cursorTime.Before(start) || cursorTime.After(end) {
			return LogPage{}, fmt.Errorf("%w: cursor is outside the requested time range", ErrInvalidLogFilter)
		}
		if cursorTime.Equal(start) {
			return LogPage{Logs: []_type.Log{}}, nil
		}
		queryEnd = cursorTime
	}

	query := buildLogQuery(teamID, pageSize+1, category, level, userIDs, message, start.UTC(), queryEnd)
	ctx, cancel := context.WithTimeout(ctx, logQueryTimeout)
	defer cancel()

	result, err := Client.QueryAPI(config.Conf.InfluxDBOrg).Query(ctx, query)
	if err != nil {
		return LogPage{}, err
	}
	defer func() {
		_ = result.Close()
	}()

	logs := make([]_type.Log, 0, pageSize+1)
	for result.Next() {
		record := result.Record()
		logRecord := _type.Log{Time: record.Time()}

		if val, ok := record.ValueByKey("user_id").(int64); ok {
			logRecord.UserID = val
		}
		if val, ok := record.ValueByKey("category").(string); ok {
			logRecord.Category = val
		}
		if val, ok := record.ValueByKey("message").(string); ok {
			logRecord.Message = val
		}
		if val, ok := record.ValueByKey("ip").(string); ok {
			logRecord.IP = val
		}
		if val, ok := record.ValueByKey("ip_country").(string); ok {
			logRecord.IPCountry = val
		}
		if val, ok := record.ValueByKey("ip_country_code").(string); ok {
			logRecord.IPCountryCode = val
		}
		if val, ok := record.ValueByKey("user_agent").(string); ok {
			logRecord.UserAgent = val
		}
		if val, ok := record.ValueByKey("level").(string); ok {
			logRecord.Level = val
		}

		logs = append(logs, logRecord)
	}
	if err = result.Err(); err != nil {
		return LogPage{}, err
	}

	page := LogPage{Logs: logs}
	if len(logs) > pageSize {
		page.HasMore = true
		page.Logs = logs[:pageSize]
		// The cursor intentionally contains only _time. Because range(stop:) is
		// exclusive, records beyond the page boundary that share this exact
		// timestamp are skipped; avoiding that rare case requires a compound key.
		page.NextCursor = encodeLogCursor(page.Logs[len(page.Logs)-1].Time)
	}
	return page, nil
}

func buildLogQuery(teamID int64, limit int, category, level string, userIDs []int64, message string, start, end time.Time) string {
	var imports string
	var beforePivot strings.Builder
	var afterPivot strings.Builder
	if category != "" && category != "all" {
		value := fluxStringLiteral(category)
		fmt.Fprintf(&beforePivot, `
	|> filter(fn: (r) => if exists r.category then r.category == %s else true)`, value)
		fmt.Fprintf(&afterPivot, `
	|> filter(fn: (r) => r["category"] == %s)`, value)
	}
	if level != "" && level != "all" {
		value := fluxStringLiteral(level)
		fmt.Fprintf(&beforePivot, `
	|> filter(fn: (r) => if exists r.level then r.level == %s else true)`, value)
		fmt.Fprintf(&afterPivot, `
	|> filter(fn: (r) => r["level"] == %s)`, value)
	}
	if len(userIDs) > 0 {
		afterPivot.WriteString(`
	|> filter(fn: (r) => (`)
		for i, uid := range userIDs {
			if i > 0 {
				afterPivot.WriteString(" or ")
			}
			fmt.Fprintf(&afterPivot, `r["user_id"] == %d`, uid)
		}
		afterPivot.WriteString("))")
	}
	if message != "" {
		imports = "import \"strings\"\n"
		fmt.Fprintf(&afterPivot, `
	|> filter(fn: (r) => strings.containsStr(v: r["message"], substr: %s))`, fluxStringLiteral(message))
	}

	// InfluxDB OSS does not support params.*, so validated values are rendered
	// as escaped Flux literals.
	return fmt.Sprintf(`%sfrom(bucket: "logs")
	|> range(start: time(v: %s), stop: time(v: %s))
	|> filter(fn: (r) => r._measurement == "logs")
	|> filter(fn: (r) => r["team_id"] == %s)%s
	|> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")%s
	|> group()
	|> sort(columns: ["_time"], desc: true)
	|> limit(n: %d)`, imports, fluxStringLiteral(start.Format(time.RFC3339Nano)),
		fluxStringLiteral(end.Format(time.RFC3339Nano)), fluxStringLiteral(strconv.FormatInt(teamID, 10)),
		beforePivot.String(), afterPivot.String(), limit)
}

func encodeLogCursor(before time.Time) string {
	return base64.RawURLEncoding.EncodeToString([]byte(before.UTC().Format(time.RFC3339Nano)))
}

func decodeLogCursor(cursor string) (time.Time, error) {
	value, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid cursor", ErrInvalidLogFilter)
	}
	before, err := time.Parse(time.RFC3339Nano, string(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid cursor", ErrInvalidLogFilter)
	}
	return before.UTC(), nil
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
