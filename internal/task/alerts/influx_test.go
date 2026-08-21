package alerttasks

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"mosona-manager/internal/_type"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api"
)

type alertCSVRow struct {
	timestamp time.Time
	value     string
	field     string
	serverID  string
}

func newAlertQueryResult(t *testing.T, valueType string, rows []alertCSVRow) *api.QueryTableResult {
	t.Helper()
	body := alertQueryCSV(t, valueType, 0, rows)
	return api.NewQueryTableResult(io.NopCloser(strings.NewReader(body)))
}

func alertQueryCSV(t *testing.T, valueType string, table int, rows []alertCSVRow) string {
	t.Helper()
	var body strings.Builder
	writer := csv.NewWriter(&body)
	for _, row := range [][]string{
		{"#datatype", "string", "long", "dateTime:RFC3339Nano", valueType, "string", "string", "string"},
		{"#group", "false", "false", "false", "false", "true", "true", "true"},
		{"#default", "_result", "", "", "", "", "", ""},
		{"", "result", "table", "_time", "_value", "_field", "_measurement", "server_id"},
	} {
		if err := writer.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range rows {
		if err := writer.Write([]string{
			"", "", strconv.Itoa(table), row.timestamp.UTC().Format(time.RFC3339Nano), row.value,
			row.field, "server_status", row.serverID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	return body.String()
}

func TestBuildAlertItemQuerySelectsBucketAndAggregation(t *testing.T) {
	end := time.Date(2026, 8, 21, 12, 34, 56, 0, time.UTC)
	tests := []struct {
		name       string
		item       string
		duration   int
		contains   []string
		notContain []string
	}{
		{
			name:     "short cpu uses raw mean",
			item:     alertItemCPU,
			duration: 15,
			contains: []string{
				`from(bucket: "server_status_raw")`,
				`r._field == "cpu"`,
				`mean(column: "_value")`,
			},
			notContain: []string{"server_status_minute", "sort(columns", "mem_used_mb"},
		},
		{
			name:     "long memory joins minute and raw tail",
			item:     alertItemMemory,
			duration: 16,
			contains: []string{
				`from(bucket: "server_status_minute")`,
				`from(bucket: "server_status_raw")`,
				`r._time <= 2026-08-21T12:32:00Z`,
				`aggregateWindow(every: 1m, fn: mean, createEmpty: false)`,
				`r._field == "mem_total_mb"`,
				`r._field == "mem_used_mb"`,
				`pivot(rowKey: ["_time"]`,
				`_field: "memory_usage"`,
			},
			notContain: []string{"sort(columns", "disk_read_iops"},
		},
		{
			name:     "status always uses raw last",
			item:     alertItemStatus,
			duration: 1440,
			contains: []string{
				`from(bucket: "server_status_raw")`,
				`r._field == "cpu"`,
				`last()`,
			},
			notContain: []string{"server_status_minute", "mean(column", "sort(columns"},
		},
		{
			name:     "long disk uses last for raw tail",
			item:     alertItemDisk,
			duration: 60,
			contains: []string{
				`r._field == "disks"`,
				`r._field == "disk_total_gb"`,
				`r._field == "disk_used_gb"`,
				`aggregateWindow(every: 1m, fn: last, createEmpty: false)`,
			},
			notContain: []string{"mean(column", "sort(columns", "rx_kib_s"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query, err := buildAlertItemQuery(test.item, map[int][]int64{test.duration: {7, 3}}, end)
			if err != nil {
				t.Fatal(err)
			}
			for _, fragment := range test.contains {
				if !strings.Contains(query, fragment) {
					t.Fatalf("query missing %q:\n%s", fragment, query)
				}
			}
			for _, fragment := range test.notContain {
				if strings.Contains(query, fragment) {
					t.Fatalf("query unexpectedly contains %q:\n%s", fragment, query)
				}
			}
			if strings.Index(query, `r.server_id == "3"`) > strings.Index(query, `r.server_id == "7"`) {
				t.Fatalf("server IDs are not sorted:\n%s", query)
			}
		})
	}
}

func TestBuildAlertItemQueryGroupsDistinctDurations(t *testing.T) {
	end := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	query, err := buildAlertItemQuery(alertItemCPU, map[int][]int64{
		60: {4, 2},
		10: {3, 1},
	}, end)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		`branch0 = from(bucket: "server_status_raw")`,
		`range(start: 2026-08-21T11:50:00Z`,
		`minute1 = from(bucket: "server_status_minute")`,
		`range(start: 2026-08-21T11:00:00Z`,
		`union(tables: [branch0, branch1])`,
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query missing %q:\n%s", fragment, query)
		}
	}
}

func TestBuildAlertItemQueriesCapsBranches(t *testing.T) {
	groups := make(map[int][]int64)
	for duration := 1; duration <= alertMaxQueryBranches*2+1; duration++ {
		groups[duration] = []int64{int64(duration)}
	}
	plans, err := buildAlertItemQueries(alertItemCPU, groups, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 {
		t.Fatalf("query plan count = %d, want 3", len(plans))
	}
	for index, wantServers := range []int{alertMaxQueryBranches, alertMaxQueryBranches, 1} {
		if got := len(plans[index].serverIDs); got != wantServers {
			t.Fatalf("plan %d server count = %d, want %d", index, got, wantServers)
		}
		if strings.Contains(plans[index].query, "branch8") {
			t.Fatalf("plan %d exceeded the branch limit:\n%s", index, plans[index].query)
		}
	}
	if _, err = buildAlertItemQuery(alertItemCPU, groups, time.Now()); err == nil {
		t.Fatal("single-query builder accepted more than the branch limit")
	}
}

func TestBuildAlertItemQueryDropsInvalidDurationGroup(t *testing.T) {
	query, err := buildAlertItemQuery(alertItemCPU, map[int][]int64{
		0:  {7},
		10: {8},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(query, `r.server_id == "7"`) || !strings.Contains(query, `r.server_id == "8"`) {
		t.Fatalf("invalid duration group was not isolated:\n%s", query)
	}
}

func TestAlertItemFieldsOnlyReturnsRequiredSeries(t *testing.T) {
	tests := map[string][]string{
		alertItemStatus:    {"cpu"},
		alertItemCPU:       {"cpu"},
		alertItemMemory:    {"mem_total_mb", "mem_used_mb"},
		alertItemDisk:      {"disks", "disk_total_gb", "disk_used_gb"},
		alertItemReadIOPS:  {"disk_read_iops"},
		alertItemWriteIOPS: {"disk_write_iops"},
		alertItemBandwidth: {"rx_kib_s", "tx_kib_s"},
	}
	for item, want := range tests {
		got, err := alertItemFields(item)
		if err != nil {
			t.Fatalf("fields for %s: %v", item, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("fields for %s = %#v, want %#v", item, got, want)
		}
	}
	if _, err := alertItemFields(alertItemExpiry); err == nil {
		t.Fatal("expiry reminder unexpectedly has InfluxDB fields")
	}
}

func TestAlertDurationGroupsPreservesServerOwnership(t *testing.T) {
	serverRules := map[int64]map[string]_type.ServerAlert{
		7:  {alertItemCPU: {ForDuration: 10}},
		8:  {alertItemCPU: {ForDuration: 20}},
		9:  {alertItemCPU: {ForDuration: 10}},
		10: {alertItemCPU: {ForDuration: 0}},
		11: {alertItemMemory: {ForDuration: 10}},
	}
	groups, requested, invalid := alertDurationGroups([]int64{11, 10, 9, 8, 7}, serverRules, alertItemCPU)
	if !reflect.DeepEqual(groups, map[int][]int64{10: {9, 7}, 20: {8}}) {
		t.Fatalf("duration groups = %#v", groups)
	}
	if !reflect.DeepEqual(requested, []int64{9, 8, 7}) {
		t.Fatalf("requested server IDs = %#v", requested)
	}
	if invalid != 1 {
		t.Fatalf("invalid duration count = %d, want 1", invalid)
	}
}

func TestLoadAlertObservationsChunksServersAndSkipsExpiryOnly(t *testing.T) {
	rules := map[int64]map[int64]map[string]_type.ServerAlert{1: {}}
	for serverID := int64(1); serverID <= 130; serverID++ {
		rules[1][serverID] = map[string]_type.ServerAlert{
			alertItemCPU: {Item: alertItemCPU, ForDuration: 60},
		}
	}
	rules[1][131] = map[string]_type.ServerAlert{
		alertItemExpiry: {Item: alertItemExpiry},
	}

	var queries []string
	executor := func(_ context.Context, query string) (*api.QueryTableResult, error) {
		queries = append(queries, query)
		return newAlertQueryResult(t, "double", nil), nil
	}
	observations := loadAlertObservations(context.Background(), rules, time.Now(), executor)

	if len(queries) != 3 {
		t.Fatalf("query count = %d, want 3", len(queries))
	}
	for index, wantFilters := range []int{128, 128, 4} {
		if got := strings.Count(queries[index], `r.server_id ==`); got != wantFilters {
			t.Fatalf("query %d server filters = %d, want %d", index, got, wantFilters)
		}
		if strings.Contains(queries[index], "expiry") || strings.Contains(queries[index], "mem_") {
			t.Fatalf("query %d contains an unrelated alert field:\n%s", index, queries[index])
		}
	}
	for serverID := int64(1); serverID <= 130; serverID++ {
		observation, loaded := observations.get(serverID, alertItemCPU)
		if !loaded || observation.present {
			t.Fatalf("server %d observation = %#v, loaded=%t", serverID, observation, loaded)
		}
	}
	if _, loaded := observations.get(131, alertItemExpiry); loaded {
		t.Fatal("expiry-only server was marked as loaded from InfluxDB")
	}
}

func TestLoadAlertObservationsIsolatesItemFailure(t *testing.T) {
	rules := map[int64]map[int64]map[string]_type.ServerAlert{
		1: {
			7: {
				alertItemCPU:    {Item: alertItemCPU, ForDuration: 10},
				alertItemMemory: {Item: alertItemMemory, ForDuration: 10},
			},
		},
	}
	wantErr := errors.New("memory query failed")
	executor := func(_ context.Context, query string) (*api.QueryTableResult, error) {
		if strings.Contains(query, `r._field == "mem_total_mb"`) {
			return nil, wantErr
		}
		return newAlertQueryResult(t, "double", []alertCSVRow{{
			timestamp: time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC),
			value:     "88.5",
			field:     "cpu",
			serverID:  "7",
		}}), nil
	}

	observations := loadAlertObservations(context.Background(), rules, time.Now(), executor)
	observation, loaded := observations.get(7, alertItemCPU)
	if !loaded || !observation.present || observation.value != 88.5 {
		t.Fatalf("CPU observation = %#v, loaded=%t", observation, loaded)
	}
	if _, loaded = observations.get(7, alertItemMemory); loaded {
		t.Fatal("failed memory query was marked as loaded")
	}
	if observations.queryFailures != 1 {
		t.Fatalf("query failures = %d, want 1", observations.queryFailures)
	}
}

func TestLoadAlertObservationsIsolatesSplitQueryFailure(t *testing.T) {
	rules := map[int64]map[int64]map[string]_type.ServerAlert{1: {}}
	for serverID := int64(1); serverID <= alertMaxQueryBranches+1; serverID++ {
		rules[1][serverID] = map[string]_type.ServerAlert{
			alertItemCPU: {Item: alertItemCPU, ForDuration: int(serverID)},
		}
	}
	queryCalls := 0
	executor := func(_ context.Context, _ string) (*api.QueryTableResult, error) {
		queryCalls++
		if queryCalls == 1 {
			return nil, errors.New("first split failed")
		}
		return newAlertQueryResult(t, "double", nil), nil
	}

	observations := loadAlertObservations(context.Background(), rules, time.Now(), executor)
	if queryCalls != 2 || observations.queryFailures != 1 {
		t.Fatalf("query calls = %d, failures = %d", queryCalls, observations.queryFailures)
	}
	for serverID := int64(1); serverID <= alertMaxQueryBranches; serverID++ {
		if _, loaded := observations.get(serverID, alertItemCPU); loaded {
			t.Fatalf("server %d from failed split was marked as loaded", serverID)
		}
	}
	if _, loaded := observations.get(alertMaxQueryBranches+1, alertItemCPU); !loaded {
		t.Fatal("server from successful split was not marked as loaded")
	}
}

func TestLoadAlertObservationsSkipsOnlyInvalidDuration(t *testing.T) {
	rules := map[int64]map[int64]map[string]_type.ServerAlert{
		1: {
			7: {alertItemCPU: {Item: alertItemCPU, ForDuration: 0}},
			8: {alertItemCPU: {Item: alertItemCPU, ForDuration: 10}},
		},
	}
	queryCalls := 0
	executor := func(_ context.Context, query string) (*api.QueryTableResult, error) {
		queryCalls++
		if strings.Contains(query, `r.server_id == "7"`) || !strings.Contains(query, `r.server_id == "8"`) {
			t.Fatalf("query contains the wrong servers:\n%s", query)
		}
		return newAlertQueryResult(t, "double", nil), nil
	}

	observations := loadAlertObservations(context.Background(), rules, time.Now(), executor)
	if queryCalls != 1 || observations.invalidDurations != 1 {
		t.Fatalf("query calls = %d, invalid durations = %d", queryCalls, observations.invalidDurations)
	}
	if _, loaded := observations.get(7, alertItemCPU); loaded {
		t.Fatal("invalid-duration rule was marked as loaded")
	}
	if _, loaded := observations.get(8, alertItemCPU); !loaded {
		t.Fatal("valid rule was not marked as loaded")
	}
}

func TestLoadAlertObservationsDoesNotQueryAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rules := map[int64]map[int64]map[string]_type.ServerAlert{
		1: {7: {alertItemCPU: {Item: alertItemCPU, ForDuration: 10}}},
	}
	called := false
	observations := loadAlertObservations(ctx, rules, time.Now(), func(context.Context, string) (*api.QueryTableResult, error) {
		called = true
		return newAlertQueryResult(t, "double", nil), nil
	})
	if called {
		t.Fatal("executor was called after loading context was canceled")
	}
	if _, loaded := observations.get(7, alertItemCPU); loaded {
		t.Fatal("canceled observation was marked as loaded")
	}
	if !observations.loadStopped {
		t.Fatal("canceled loading was not reflected in observation statistics")
	}
}

func TestQueryAlertItemParsesStatusPresence(t *testing.T) {
	executor := func(context.Context, string) (*api.QueryTableResult, error) {
		return newAlertQueryResult(t, "double", []alertCSVRow{{
			timestamp: time.Date(2026, 8, 21, 12, 0, 0, 123, time.UTC),
			value:     "42",
			field:     "cpu",
			serverID:  "7",
		}}), nil
	}
	observations, err := queryAlertItem(context.Background(), alertItemStatus, "query", executor)
	if err != nil {
		t.Fatal(err)
	}
	if got := observations[7]; !got.present {
		t.Fatalf("status observation = %#v", got)
	}
}

func TestQueryAlertItemAggregatesModernDiskSnapshots(t *testing.T) {
	first := time.Date(2026, 8, 21, 11, 59, 0, 0, time.UTC)
	last := first.Add(time.Minute)
	executor := func(context.Context, string) (*api.QueryTableResult, error) {
		return newAlertQueryResult(t, "string", []alertCSVRow{
			{timestamp: first, value: `[{"mp":"/","total_gb":100,"used_gb":50},{"mp":"/data","total_gb":200,"used_gb":100}]`, field: "disks", serverID: "7"},
			{timestamp: last, value: `[{"mp":"/","total_gb":100,"used_gb":90},{"mp":"/data","total_gb":200,"used_gb":100}]`, field: "disks", serverID: "7"},
		}), nil
	}

	observations, err := queryAlertItem(context.Background(), alertItemDisk, "query", executor)
	if err != nil {
		t.Fatal(err)
	}
	observation := observations[7]
	if !observation.present || observation.value != 70 || observation.mountPoint != "/" {
		t.Fatalf("disk observation = %#v", observation)
	}
}

func TestQueryAlertItemAggregatesLegacyDiskFields(t *testing.T) {
	timestamp := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	executor := func(context.Context, string) (*api.QueryTableResult, error) {
		return newAlertQueryResult(t, "double", []alertCSVRow{
			{timestamp: timestamp, value: "100", field: "disk_total_gb", serverID: "7"},
			{timestamp: timestamp, value: "80", field: "disk_used_gb", serverID: "7"},
		}), nil
	}

	observations, err := queryAlertItem(context.Background(), alertItemDisk, "query", executor)
	if err != nil {
		t.Fatal(err)
	}
	observation := observations[7]
	if !observation.present || observation.value != 80 || observation.mountPoint != "/" {
		t.Fatalf("legacy disk observation = %#v", observation)
	}
}

func TestQueryAlertItemDiskTieBreakNormalizesEmptyMountPoint(t *testing.T) {
	timestamp := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	executor := func(context.Context, string) (*api.QueryTableResult, error) {
		return newAlertQueryResult(t, "string", []alertCSVRow{{
			timestamp: timestamp,
			value:     `[{"mp":"/z","total_gb":100,"used_gb":50},{"mp":"","total_gb":200,"used_gb":100}]`,
			field:     "disks",
			serverID:  "7",
		}}), nil
	}

	observations, err := queryAlertItem(context.Background(), alertItemDisk, "query", executor)
	if err != nil {
		t.Fatal(err)
	}
	observation := observations[7]
	if !observation.present || observation.value != 50 || observation.mountPoint != "/" {
		t.Fatalf("tied disk observation = %#v", observation)
	}
}

func TestQueryAlertItemModernDiskOverridesLegacyAtSameTimestamp(t *testing.T) {
	timestamp := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	legacy := alertQueryCSV(t, "double", 0, []alertCSVRow{
		{timestamp: timestamp, value: "100", field: "disk_total_gb", serverID: "7"},
		{timestamp: timestamp, value: "90", field: "disk_used_gb", serverID: "7"},
	})
	modern := alertQueryCSV(t, "string", 1, []alertCSVRow{{
		timestamp: timestamp,
		value:     `[{"mp":"/","total_gb":100,"used_gb":40}]`,
		field:     "disks",
		serverID:  "7",
	}})

	for _, test := range []struct {
		name string
		body string
	}{
		{name: "legacy then modern", body: legacy + "\n" + modern},
		{name: "modern then legacy", body: modern + "\n" + legacy},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := func(context.Context, string) (*api.QueryTableResult, error) {
				return api.NewQueryTableResult(io.NopCloser(strings.NewReader(test.body))), nil
			}
			observations, err := queryAlertItem(context.Background(), alertItemDisk, "query", executor)
			if err != nil {
				t.Fatal(err)
			}
			observation := observations[7]
			if !observation.present || observation.value != 40 || observation.mountPoint != "/" {
				t.Fatalf("mixed disk observation = %#v", observation)
			}
		})
	}
}
