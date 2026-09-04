package alerttasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/influx"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api"
)

const (
	alertServerBatchSize  = 64
	alertMaxQueryBranches = 8
	alertRawWindowMinutes = 15
	alertRawTailDuration  = 2 * time.Minute
	alertQueryTimeout     = 30 * time.Second
	alertLoadTimeout      = 2 * time.Minute

	alertItemStatus    = "status"
	alertItemCPU       = "cpu_usage"
	alertItemMemory    = "memory_usage"
	alertItemDisk      = "disk_usage"
	alertItemReadIOPS  = "read_iops"
	alertItemWriteIOPS = "write_iops"
	alertItemBandwidth = "bandwidth"
	alertItemExpiry    = "expiry_reminder"
)

var influxAlertItems = []string{
	alertItemStatus,
	alertItemCPU,
	alertItemMemory,
	alertItemDisk,
	alertItemReadIOPS,
	alertItemWriteIOPS,
	alertItemBandwidth,
}

type alertQueryExecutor func(context.Context, string) (*api.QueryTableResult, error)

type alertItemQueryPlan struct {
	query     string
	serverIDs []int64
}

func executeAlertQuery(ctx context.Context, query string) (*api.QueryTableResult, error) {
	return influx.Client.QueryAPI(config.Conf.InfluxDBOrg).Query(ctx, query)
}

func loadAlertObservations(
	ctx context.Context,
	rulesByTeam map[int64]map[int64]map[string]_type.ServerAlert,
	end time.Time,
	executor alertQueryExecutor,
) *alertObservationSet {
	observations := newAlertObservationSet()
	serverRules := make(map[int64]map[string]_type.ServerAlert)
	for _, teamRules := range rulesByTeam {
		for serverID, rules := range teamRules {
			serverRules[serverID] = rules
		}
	}

	serverIDs := make([]int64, 0, len(serverRules))
	for serverID := range serverRules {
		serverIDs = append(serverIDs, serverID)
	}
	sort.Slice(serverIDs, func(i, j int) bool { return serverIDs[i] < serverIDs[j] })

	for offset := 0; offset < len(serverIDs); offset += alertServerBatchSize {
		if ctx.Err() != nil {
			observations.loadStopped = true
			log.Printf("alert observation loading stopped: %v", ctx.Err())
			return observations
		}
		endOffset := min(offset+alertServerBatchSize, len(serverIDs))
		batch := serverIDs[offset:endOffset]

		for _, item := range influxAlertItems {
			if ctx.Err() != nil {
				observations.loadStopped = true
				log.Printf("alert observation loading stopped: %v", ctx.Err())
				return observations
			}
			durationGroups, requestedIDs, invalidDurations := alertDurationGroups(batch, serverRules, item)
			if invalidDurations > 0 {
				observations.invalidDurations += invalidDurations
				log.Printf("skipping %d %s alert rules with non-positive duration", invalidDurations, item)
			}
			if len(requestedIDs) == 0 {
				continue
			}
			queryPlans, err := buildAlertItemQueries(item, durationGroups, end)
			if err != nil {
				log.Printf("failed to build alert query for %s: %v", item, err)
				continue
			}

			for _, queryPlan := range queryPlans {
				if ctx.Err() != nil {
					observations.loadStopped = true
					log.Printf("alert observation loading stopped: %v", ctx.Err())
					return observations
				}
				queryCtx, cancel := context.WithTimeout(ctx, alertQueryTimeout)
				values, err := queryAlertItem(queryCtx, item, queryPlan.query, executor)
				cancel()
				if err != nil {
					observations.queryFailures++
					log.Printf("failed to query %s alert observations for %d servers: %v", item, len(queryPlan.serverIDs), err)
					continue
				}

				for _, serverID := range queryPlan.serverIDs {
					key := alertObservationKey{serverID: serverID, item: item}
					observations.loaded[key] = struct{}{}
					if observation, ok := values[serverID]; ok {
						observations.values[key] = observation
					}
				}
			}
		}
	}

	return observations
}

func alertDurationGroups(
	serverIDs []int64,
	serverRules map[int64]map[string]_type.ServerAlert,
	item string,
) (map[int][]int64, []int64, int) {
	groups := make(map[int][]int64)
	requested := make([]int64, 0, len(serverIDs))
	invalidDurations := 0
	for _, serverID := range serverIDs {
		rule, ok := serverRules[serverID][item]
		if !ok {
			continue
		}
		if rule.ForDuration <= 0 {
			invalidDurations++
			continue
		}
		groups[rule.ForDuration] = append(groups[rule.ForDuration], serverID)
		requested = append(requested, serverID)
	}
	return groups, requested, invalidDurations
}

func queryAlertItem(
	ctx context.Context,
	item string,
	query string,
	executor alertQueryExecutor,
) (map[int64]alertObservation, error) {
	result, err := executor(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = result.Close() }()

	if item == alertItemDisk {
		return parseDiskObservations(result)
	}

	observations := make(map[int64]alertObservation)
	for result.Next() {
		record := result.Record()
		serverID, ok := alertRecordServerID(record.ValueByKey("server_id"))
		if !ok {
			continue
		}
		if item == alertItemStatus {
			observations[serverID] = alertObservation{present: true}
			continue
		}
		value, ok := alertFloat(record.Value())
		if !ok {
			continue
		}
		observations[serverID] = alertObservation{present: true, value: value}
	}
	if err := result.Err(); err != nil {
		return nil, err
	}
	return observations, nil
}

type alertDiskSnapshot struct {
	status *_type.ServerStatusType
	modern bool
}

type alertDiskStat struct {
	sum   float64
	count int
}

func parseDiskObservations(result *api.QueryTableResult) (map[int64]alertObservation, error) {
	snapshots := make(map[int64]map[time.Time]*alertDiskSnapshot)
	for result.Next() {
		record := result.Record()
		serverID, ok := alertRecordServerID(record.ValueByKey("server_id"))
		if !ok {
			continue
		}
		if snapshots[serverID] == nil {
			snapshots[serverID] = make(map[time.Time]*alertDiskSnapshot)
		}
		snapshot := snapshots[serverID][record.Time()]
		if snapshot == nil {
			snapshot = &alertDiskSnapshot{status: &_type.ServerStatusType{}}
			snapshots[serverID][record.Time()] = snapshot
		}

		switch record.Field() {
		case "disks":
			value, ok := record.Value().(string)
			if !ok || value == "" {
				continue
			}
			var disks []_type.DiskInfo
			if err := json.Unmarshal([]byte(value), &disks); err != nil {
				continue
			}
			snapshot.status.Disks = disks
			snapshot.modern = true
		case "disk_total_gb", "disk_used_gb":
			if !snapshot.modern {
				influx.ParseDisksField(snapshot.status, record.Field(), record.Value())
			}
		}
	}
	if err := result.Err(); err != nil {
		return nil, err
	}

	observations := make(map[int64]alertObservation)
	for serverID, serverSnapshots := range snapshots {
		perDisk := make(map[string]*alertDiskStat)
		for _, snapshot := range serverSnapshots {
			for _, disk := range snapshot.status.Disks {
				if disk.TotalGB <= 0 {
					continue
				}
				mountPoint := disk.MountPoint
				if mountPoint == "" {
					mountPoint = "/"
				}
				stat := perDisk[mountPoint]
				if stat == nil {
					stat = &alertDiskStat{}
					perDisk[mountPoint] = stat
				}
				stat.sum += disk.UsedGB / disk.TotalGB * 100
				stat.count++
			}
		}

		observation := alertObservation{}
		for mountPoint, stat := range perDisk {
			if stat.count == 0 {
				continue
			}
			average := stat.sum / float64(stat.count)
			if !observation.present || average > observation.value ||
				(average == observation.value && mountPoint < observation.mountPoint) {
				observation.present = true
				observation.value = average
				observation.mountPoint = mountPoint
			}
		}
		if observation.present {
			observations[serverID] = observation
		}
	}
	return observations, nil
}

func alertRecordServerID(value interface{}) (int64, bool) {
	serverIDString, ok := value.(string)
	if !ok {
		return 0, false
	}
	serverID, err := strconv.ParseInt(serverIDString, 10, 64)
	return serverID, err == nil
}

func alertFloat(value interface{}) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case int64:
		return float64(value), true
	case uint64:
		return float64(value), true
	default:
		return 0, false
	}
}

func buildAlertItemQueries(
	item string,
	durationGroups map[int][]int64,
	end time.Time,
) ([]alertItemQueryPlan, error) {
	if _, err := alertItemFields(item); err != nil {
		return nil, err
	}
	durations := sortedValidAlertDurations(durationGroups)
	if len(durations) == 0 {
		return nil, nil
	}

	plans := make([]alertItemQueryPlan, 0, (len(durations)+alertMaxQueryBranches-1)/alertMaxQueryBranches)
	for offset := 0; offset < len(durations); offset += alertMaxQueryBranches {
		endOffset := min(offset+alertMaxQueryBranches, len(durations))
		queryGroups := make(map[int][]int64, endOffset-offset)
		serverIDs := make([]int64, 0)
		for _, duration := range durations[offset:endOffset] {
			queryGroups[duration] = durationGroups[duration]
			serverIDs = append(serverIDs, durationGroups[duration]...)
		}
		sort.Slice(serverIDs, func(i, j int) bool { return serverIDs[i] < serverIDs[j] })
		query, err := buildAlertItemQuery(item, queryGroups, end)
		if err != nil {
			return nil, err
		}
		plans = append(plans, alertItemQueryPlan{query: query, serverIDs: serverIDs})
	}
	return plans, nil
}

func buildAlertItemQuery(item string, durationGroups map[int][]int64, end time.Time) (string, error) {
	fields, err := alertItemFields(item)
	if err != nil {
		return "", err
	}

	durations := sortedValidAlertDurations(durationGroups)
	if len(durations) == 0 {
		return "", fmt.Errorf("no valid %s alert duration groups", item)
	}
	if len(durations) > alertMaxQueryBranches {
		return "", fmt.Errorf("%s alert query has %d branches, maximum is %d", item, len(durations), alertMaxQueryBranches)
	}

	definitions := make([]string, 0, len(durations)*3)
	branches := make([]string, 0, len(durations))
	for index, duration := range durations {
		ids := append([]int64(nil), durationGroups[duration]...)
		slices.Sort(ids)
		prelude, pipeline := buildAlertItemBranch(index, item, fields, ids, duration, end)
		definitions = append(definitions, prelude...)
		if len(durations) == 1 {
			definitions = append(definitions, pipeline)
			break
		}
		branchName := fmt.Sprintf("branch%d", index)
		definitions = append(definitions, fmt.Sprintf("%s = %s", branchName, pipeline))
		branches = append(branches, branchName)
	}
	if len(branches) > 0 {
		definitions = append(definitions, fmt.Sprintf("union(tables: [%s])", strings.Join(branches, ", ")))
	}
	return strings.Join(definitions, "\n\n"), nil
}

func sortedValidAlertDurations(durationGroups map[int][]int64) []int {
	durations := make([]int, 0, len(durationGroups))
	for duration := range durationGroups {
		if duration > 0 {
			durations = append(durations, duration)
		}
	}
	sort.Ints(durations)
	return durations
}

func buildAlertItemBranch(
	index int,
	item string,
	fields []string,
	serverIDs []int64,
	duration int,
	end time.Time,
) (prelude []string, pipeline string) {
	start := end.Add(-time.Duration(duration) * time.Minute)
	if item == alertItemStatus || duration <= alertRawWindowMinutes {
		source := buildAlertFluxSource("server_status_raw", start, end, serverIDs, fields, true)
		return nil, aggregateAlertFlux(item, source)
	}

	// Minute downsampling intentionally trails real time; aggregate the recent raw
	// tail to the same resolution before combining both segments.
	rawStart := end.Truncate(time.Minute).Add(-alertRawTailDuration)
	minuteName := fmt.Sprintf("minute%d", index)
	rawName := fmt.Sprintf("raw%d", index)
	minuteSource := buildAlertFluxSource("server_status_minute", start, end, serverIDs, fields, true) +
		fmt.Sprintf("\n  |> filter(fn: (r) => r._time <= %s)", formatAlertFluxTime(rawStart))
	rawSource := buildAlertFluxSource("server_status_raw", rawStart, end, serverIDs, fields, false)
	if item == alertItemDisk {
		rawSource += "\n  |> aggregateWindow(every: 1m, fn: last, createEmpty: false)"
	} else {
		rawSource += "\n  |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)"
	}
	combined := fmt.Sprintf("union(tables: [%s, %s])", minuteName, rawName)
	return []string{
		fmt.Sprintf("%s = %s", minuteName, minuteSource),
		fmt.Sprintf("%s = %s", rawName, rawSource),
	}, aggregateAlertFlux(item, combined)
}

func buildAlertFluxSource(
	bucket string,
	start time.Time,
	end time.Time,
	serverIDs []int64,
	fields []string,
	strictStart bool,
) string {
	serverFilters := make([]string, 0, len(serverIDs))
	for _, serverID := range serverIDs {
		serverFilters = append(serverFilters, fmt.Sprintf(`r.server_id == "%d"`, serverID))
	}
	fieldFilters := make([]string, 0, len(fields))
	for _, field := range fields {
		fieldFilters = append(fieldFilters, fmt.Sprintf(`r._field == "%s"`, field))
	}

	query := fmt.Sprintf(`from(bucket: "%s")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status" and (%s) and (%s))`,
		bucket,
		formatAlertFluxTime(start),
		formatAlertFluxTime(end),
		strings.Join(serverFilters, " or "),
		strings.Join(fieldFilters, " or "),
	)
	if strictStart {
		query += fmt.Sprintf("\n  |> filter(fn: (r) => r._time > %s)", formatAlertFluxTime(start))
	}
	return query
}

func aggregateAlertFlux(item string, source string) string {
	switch item {
	case alertItemStatus:
		return source + `
  |> group(columns: ["server_id"])
  |> last()`
	case alertItemCPU, alertItemReadIOPS, alertItemWriteIOPS:
		return source + `
  |> group(columns: ["server_id", "_field"])
  |> mean(column: "_value")`
	case alertItemMemory:
		return source + `
  |> group(columns: ["server_id"])
  |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
  |> filter(fn: (r) => exists r.mem_used_mb and exists r.mem_total_mb and r.mem_total_mb > 0.0)
  |> map(fn: (r) => ({r with _field: "memory_usage", _value: r.mem_used_mb / r.mem_total_mb * 100.0}))
  |> group(columns: ["server_id", "_field"])
  |> mean(column: "_value")`
	case alertItemBandwidth:
		return source + `
  |> group(columns: ["server_id"])
  |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
  |> filter(fn: (r) => exists r.rx_kib_s and exists r.tx_kib_s)
  |> map(fn: (r) => ({r with _field: "bandwidth", _value: (r.rx_kib_s + r.tx_kib_s) * 8.0 / 1024.0}))
  |> group(columns: ["server_id", "_field"])
  |> mean(column: "_value")`
	case alertItemDisk:
		return source
	default:
		panic("unsupported alert item: " + item)
	}
}

func alertItemFields(item string) ([]string, error) {
	switch item {
	case alertItemStatus, alertItemCPU:
		return []string{"cpu"}, nil
	case alertItemMemory:
		return []string{"mem_total_mb", "mem_used_mb"}, nil
	case alertItemDisk:
		return []string{"disks", "disk_total_gb", "disk_used_gb"}, nil
	case alertItemReadIOPS:
		return []string{"disk_read_iops"}, nil
	case alertItemWriteIOPS:
		return []string{"disk_write_iops"}, nil
	case alertItemBandwidth:
		return []string{"rx_kib_s", "tx_kib_s"}, nil
	default:
		return nil, fmt.Errorf("unsupported influx alert item %q", item)
	}
}

func formatAlertFluxTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
