package influx

import (
	"context"
	"encoding/json"
	"errors"
	"mosona-manager/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

func TestBuildLogQueryUsesCursorLimitAndEscapesFluxLiterals(t *testing.T) {
	start := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	message := `") |> yield(name: "injected") // ${ 1 + 1 }`
	query := buildLogQuery(12, 21, "server", "high", []int64{7, 8}, message, start, end)
	if strings.Contains(query, message) {
		t.Fatalf("user-controlled value was embedded unescaped in Flux source: %s", query)
	}
	for _, fragment := range []string{
		`range(start: time(v: "2026-07-01T00:00:00Z"), stop: time(v: "2026-07-02T00:00:00Z"))`,
		`r["team_id"] == "12"`,
		`if exists r.category then r.category == "server" else true`,
		`if exists r.level then r.level == "high" else true`,
		`r["category"] == "server"`,
		`r["level"] == "high"`,
		`r["user_id"] == 7`,
		`substr: "\") |> yield(name: \"injected\") // \${ 1 + 1 }"`,
		`strings.containsStr(`,
		`group()`,
		`limit(n: 21)`,
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("Flux query missing %q: %s", fragment, query)
		}
	}
	if strings.Contains(query, "offset:") || strings.Contains(query, "count(") {
		t.Fatalf("cursor query contains offset or count scan: %s", query)
	}
	if strings.Index(query, "if exists r.category") > strings.Index(query, "pivot(") {
		t.Fatalf("category tag filter must run before pivot: %s", query)
	}
}

func TestGetLogsUsesOneQueryAndBuildsNextCursor(t *testing.T) {
	const csvResult = `#datatype,string,long,dateTime:RFC3339Nano,long,string,string,string,string,string,string,string
#group,false,false,false,false,false,false,false,false,false,false,false
#default,_result,,,,,,,,,,
,result,table,_time,user_id,category,message,ip,ip_country,ip_country_code,user_agent,level
,,0,2026-07-02T10:00:00Z,7,server,newest,127.0.0.1,Private Network,UN,test-agent,high
,,0,2026-07-02T09:00:00Z,8,server,second,127.0.0.1,Private Network,UN,test-agent,medium
,,0,2026-07-02T08:00:00Z,9,server,extra,127.0.0.1,Private Network,UN,test-agent,low

`
	queries := make([]string, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode query request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		queries = append(queries, payload.Query)
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(csvResult))
	}))
	defer server.Close()

	previousClient := Client
	previousOrg := config.Conf.InfluxDBOrg
	testClient := influxdb2.NewClient(server.URL, "test-token")
	Client = testClient
	config.Conf.InfluxDBOrg = "test-org"
	t.Cleanup(func() {
		testClient.Close()
		Client = previousClient
		config.Conf.InfluxDBOrg = previousOrg
	})

	start := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.July, 3, 0, 0, 0, 0, time.UTC)
	page, err := GetLogs(context.Background(), 12, 2, "", "", nil, "", start, end, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 {
		t.Fatalf("query request count = %d, want 1", len(queries))
	}
	if !strings.Contains(queries[0], `stop: time(v: "2026-07-03T00:00:00Z")`) {
		t.Fatalf("first-page query did not use requested end: %s", queries[0])
	}
	if !page.HasMore || len(page.Logs) != 2 || page.NextCursor == "" {
		t.Fatalf("unexpected page metadata: %#v", page)
	}
	if page.Logs[0].UserID != 7 || page.Logs[0].Message != "newest" || page.Logs[0].Level != "high" {
		t.Fatalf("unexpected first log: %#v", page.Logs[0])
	}
	cursorTime, err := decodeLogCursor(page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if !cursorTime.Equal(page.Logs[1].Time) {
		t.Fatalf("cursor time = %s, want %s", cursorTime, page.Logs[1].Time)
	}
}

func TestGetLogsAppliesCursorAndValidatesBounds(t *testing.T) {
	queries := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode query request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		queries = append(queries, payload.Query)
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	previousClient := Client
	previousOrg := config.Conf.InfluxDBOrg
	testClient := influxdb2.NewClient(server.URL, "test-token")
	Client = testClient
	config.Conf.InfluxDBOrg = "test-org"
	t.Cleanup(func() {
		testClient.Close()
		Client = previousClient
		config.Conf.InfluxDBOrg = previousOrg
	})

	start := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.July, 3, 0, 0, 0, 0, time.UTC)
	cursorTime := time.Date(2026, time.July, 2, 9, 0, 0, 123, time.UTC)
	if _, err := GetLogs(context.Background(), 12, 20, "", "", nil, "", start, end, encodeLogCursor(cursorTime)); err != nil {
		t.Fatalf("GetLogs with in-range cursor: %v", err)
	}
	if len(queries) != 1 || !strings.Contains(queries[0], `stop: time(v: "2026-07-02T09:00:00.000000123Z")`) {
		t.Fatalf("cursor was not applied as the query stop: %#v", queries)
	}

	// InfluxDB rejects range(start: X, stop: X), so this boundary is handled
	// locally as an empty page without issuing a query.
	emptyPage, err := GetLogs(context.Background(), 12, 20, "", "", nil, "", start, end, encodeLogCursor(start))
	if err != nil {
		t.Fatalf("GetLogs rejected cursor equal to start: %v", err)
	}
	if emptyPage.Logs == nil || len(emptyPage.Logs) != 0 || emptyPage.HasMore || emptyPage.NextCursor != "" {
		t.Fatalf("start-boundary cursor returned %#v, want an empty final page", emptyPage)
	}
	if len(queries) != 1 {
		t.Fatalf("start-boundary cursor reached InfluxDB; query count = %d, want 1", len(queries))
	}

	for _, outside := range []time.Time{start.Add(-time.Nanosecond), end.Add(time.Nanosecond)} {
		if _, err := GetLogs(context.Background(), 12, 20, "", "", nil, "", start, end, encodeLogCursor(outside)); !errors.Is(err, ErrInvalidLogFilter) {
			t.Fatalf("out-of-range cursor %s error = %v, want ErrInvalidLogFilter", outside, err)
		}
	}
	if len(queries) != 1 {
		t.Fatalf("out-of-range cursors reached InfluxDB; query count = %d, want 1", len(queries))
	}
}

func TestParseLogTimeRange(t *testing.T) {
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	start, end, err := ParseLogTimeRange("", "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if !end.Equal(now) || !start.Equal(now.Add(-defaultLogRange)) {
		t.Fatalf("default range = %s..%s", start, end)
	}

	for _, test := range []struct {
		name    string
		start   string
		end     string
		message string
	}{
		{name: "invalid start", start: "not-a-time"},
		{name: "reverse", start: now.Format(time.RFC3339), end: now.Add(-time.Hour).Format(time.RFC3339)},
		{name: "over maximum", start: now.Add(-maxLogRange - time.Second).Format(time.RFC3339)},
		{name: "message over maximum", start: now.Add(-maxLogMessageSearchRange - time.Second).Format(time.RFC3339), message: "needle"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := ParseLogTimeRange(test.start, test.end, test.message, now); !errors.Is(err, ErrInvalidLogFilter) {
				t.Fatalf("error = %v, want ErrInvalidLogFilter", err)
			}
		})
	}
}

func TestLogCursorRoundTrip(t *testing.T) {
	want := time.Date(2026, time.August, 21, 12, 34, 56, 789, time.FixedZone("test", 8*60*60))
	cursor := encodeLogCursor(want)
	got, err := decodeLogCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Fatalf("decoded cursor = %s, want %s", got, want)
	}
	if _, err = decodeLogCursor("not-a-valid-cursor"); !errors.Is(err, ErrInvalidLogFilter) {
		t.Fatalf("invalid cursor error = %v, want ErrInvalidLogFilter", err)
	}
}

func TestValidateLogFilters(t *testing.T) {
	if err := ValidateLogFilters("security", "high", strings.Repeat("x", maxLogMessageFilterLength)); err != nil {
		t.Fatalf("valid filters rejected: %v", err)
	}
	for _, test := range []struct {
		name     string
		category string
		level    string
		message  string
	}{
		{name: "category", category: `server") or true`},
		{name: "level", level: "critical"},
		{name: "message length", message: strings.Repeat("x", maxLogMessageFilterLength+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateLogFilters(test.category, test.level, test.message); err == nil {
				t.Fatal("invalid filters were accepted")
			}
		})
	}
}

func TestValidateLogPageSize(t *testing.T) {
	for _, pageSize := range []int{0, -1, maxLogPageSize + 1} {
		if err := ValidateLogPageSize(pageSize); !errors.Is(err, ErrInvalidLogFilter) {
			t.Fatalf("page size %d error = %v, want ErrInvalidLogFilter", pageSize, err)
		}
	}
	if err := ValidateLogPageSize(maxLogPageSize); err != nil {
		t.Fatalf("valid page size rejected: %v", err)
	}
}
