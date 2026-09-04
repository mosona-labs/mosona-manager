//go:build integration

package alerttasks

// End-to-end regression test for alert observation loading against a real
// InfluxDB 2 instance. It reproduces the production layout (raw + minute
// buckets, minute downsampling, single-duration rules) and verifies that
// loadAlertObservations produces usable observations for every alert item.
//
// Requires a reachable InfluxDB container, e.g.:
//
//	docker run -d --name mm-alerts-influx -p 58087:8086 influxdb:2.7
//	docker exec mm-alerts-influx influx setup --username mm \
//	  --password mmpass1234 --org mm_org --bucket mm_bucket \
//	  --token mm_alerts_token --force
//
// Run with: go test -tags integration ./internal/task/alerts/ \
//   -run TestAlertObservationsIntegration
// (plus the usual POSTGRES_*/INFLUXDB_*/REDIS_* env vars required by config)

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/domain"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/influx"
)

const (
	integrationAlertsURL   = "http://localhost:58087"
	integrationAlertsOrg   = "mm_org"
	integrationAlertsToken = "mm_alerts_token"
)

func TestAlertObservationsIntegration(t *testing.T) {
	client := influxdb2.NewClient(integrationAlertsURL, integrationAlertsToken)
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	ok, err := client.Ping(pingCtx)
	pingCancel()
	if err != nil || !ok {
		t.Skipf("InfluxDB test container not reachable at %s: %v", integrationAlertsURL, err)
	}
	defer client.Close()

	// Point the package-level client and org at the test instance so the
	// production executeAlertQuery works unchanged.
	oldClient := influx.Client
	influx.Client = client
	defer func() { influx.Client = oldClient }()
	oldOrg := config.Conf.InfluxDBOrg
	config.Conf.InfluxDBOrg = integrationAlertsOrg
	defer func() { config.Conf.InfluxDBOrg = oldOrg }()

	ctx := context.Background()
	bucketsAPI := client.BucketsAPI()
	org, err := client.OrganizationsAPI().FindOrganizationByName(ctx, integrationAlertsOrg)
	if err != nil {
		t.Fatalf("find org: %v", err)
	}
	for _, bucket := range []string{"server_status_raw", "server_status_minute"} {
		if existing, err := bucketsAPI.FindBucketByName(ctx, bucket); err != nil || existing == nil {
			_, err = bucketsAPI.CreateBucket(ctx, &domain.Bucket{Name: bucket, OrgID: org.Id})
			if err != nil {
				t.Fatalf("create bucket %s: %v", bucket, err)
			}
		}
		if err := client.DeleteAPI().DeleteWithName(ctx, integrationAlertsOrg, bucket,
			time.Now().Add(-24*time.Hour), time.Now().Add(time.Hour), ""); err != nil {
			t.Fatalf("clean bucket %s: %v", bucket, err)
		}
	}

	// Server 7: hot metrics sampled every 30 seconds for the last 40 minutes.
	// Server 8: no data (offline).
	now := time.Now().UTC()
	writer := client.WriteAPIBlocking(integrationAlertsOrg, "server_status_raw")
	for at := now.Add(-40 * time.Minute); at.Before(now); at = at.Add(30 * time.Second) {
		at = at.Truncate(time.Second)
		disksJSON, _ := json.Marshal([]_type.DiskInfo{
			{MountPoint: "/", TotalGB: 100, UsedGB: 90},
			{MountPoint: "/data", TotalGB: 200, UsedGB: 60},
		})
		point := influxdb2.NewPoint(
			"server_status",
			map[string]string{"server_id": "7"},
			map[string]interface{}{
				"cpu":             95.0,
				"mem_total_mb":    1000.0,
				"mem_used_mb":     900.0,
				"swap_total_mb":   512.0,
				"swap_used_mb":    0.0,
				"disks":           string(disksJSON),
				"disk_read_iops":  1500.0,
				"disk_write_iops": 1500.0,
				"rx_kib_s":        8192.0,
				"tx_kib_s":        8192.0,
				"tcp_total":       10,
				"udp_total":       5,
			},
			at,
		)
		if err := writer.WritePoint(ctx, point); err != nil {
			t.Fatalf("write raw point at %v: %v", at, err)
		}
	}

	// Replay the production minute downsample (status_downsample.go) over the
	// seeded window so the minute bucket mirrors the real layout.
	downsample := fmt.Sprintf(`
numeric = from(bucket: "server_status_raw")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status" and (r._field == "cpu" or r._field == "mem_total_mb" or r._field == "mem_used_mb" or r._field == "swap_total_mb" or r._field == "swap_used_mb" or r._field == "disk_read_kib_s" or r._field == "disk_write_kib_s" or r._field == "disk_read_iops" or r._field == "disk_write_iops" or r._field == "rx_kib_s" or r._field == "tx_kib_s"))
  |> group(columns: ["server_id", "_measurement", "_field"])
  |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)
  |> to(bucket: "server_status_minute", org: %q)

state = from(bucket: "server_status_raw")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status" and (r._field == "disks" or r._field == "disk_total_gb" or r._field == "disk_used_gb" or r._field == "tcp_total" or r._field == "udp_total"))
  |> group(columns: ["server_id", "_measurement", "_field"])
  |> aggregateWindow(every: 1m, fn: last, createEmpty: false)
  |> to(bucket: "server_status_minute", org: %q)
`,
		now.Add(-40*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339), integrationAlertsOrg,
		now.Add(-40*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339), integrationAlertsOrg)
	if _, err := client.QueryAPI(integrationAlertsOrg).Query(ctx, downsample); err != nil {
		t.Fatalf("downsample: %v", err)
	}

	// Every item uses one shared duration, which is the shape that used to
	// generate union(tables: [branch0]) and fail on every query.
	metricRules := func(duration int) map[string]_type.ServerAlert {
		return map[string]_type.ServerAlert{
			alertItemStatus:    {Item: alertItemStatus, ForDuration: duration},
			alertItemCPU:       {Item: alertItemCPU, Threshold: 80, ForDuration: duration},
			alertItemMemory:    {Item: alertItemMemory, Threshold: 80, ForDuration: duration},
			alertItemDisk:      {Item: alertItemDisk, Threshold: 80, ForDuration: duration},
			alertItemReadIOPS:  {Item: alertItemReadIOPS, Threshold: 1000, ForDuration: duration},
			alertItemWriteIOPS: {Item: alertItemWriteIOPS, Threshold: 1000, ForDuration: duration},
			alertItemBandwidth: {Item: alertItemBandwidth, Threshold: 100, ForDuration: duration},
		}
	}
	rules := map[int64]map[int64]map[string]_type.ServerAlert{
		1: {
			7: metricRules(10),
			8: {alertItemStatus: {Item: alertItemStatus, ForDuration: 10}},
		},
	}

	loadCtx, loadCancel := context.WithTimeout(context.Background(), alertLoadTimeout)
	observations := loadAlertObservations(loadCtx, rules, time.Now(), executeAlertQuery)
	loadCancel()

	if observations.queryFailures != 0 || observations.invalidDurations != 0 || observations.loadStopped {
		t.Fatalf("observation loading degraded: failures=%d invalidDurations=%d loadStopped=%t",
			observations.queryFailures, observations.invalidDurations, observations.loadStopped)
	}

	offline, loaded := observations.get(8, alertItemStatus)
	if !loaded || offline.present {
		t.Fatalf("offline server status observation = %#v loaded=%t, want loaded and absent", offline, loaded)
	}

	wantValues := map[string]struct {
		present bool
		value   float64
	}{
		alertItemStatus:    {present: true},
		alertItemCPU:       {present: true, value: 95},
		alertItemMemory:    {present: true, value: 90},
		alertItemDisk:      {present: true, value: 90},
		alertItemReadIOPS:  {present: true, value: 1500},
		alertItemWriteIOPS: {present: true, value: 1500},
		alertItemBandwidth: {present: true, value: 128},
	}
	for item, want := range wantValues {
		observation, loaded := observations.get(7, item)
		if !loaded || observation.present != want.present {
			t.Fatalf("server 7 %s observation = %#v loaded=%t, want present=%t loaded=true", item, observation, loaded, want.present)
		}
		if item != alertItemStatus && observation.value != want.value {
			t.Fatalf("server 7 %s observation = %#v, want value %.2f", item, observation, want.value)
		}
	}

	// The minute-bucket path must also work for a single long-duration rule.
	longRules := map[int64]map[int64]map[string]_type.ServerAlert{
		1: {7: {
			alertItemCPU:  {Item: alertItemCPU, Threshold: 80, ForDuration: 30},
			alertItemDisk: {Item: alertItemDisk, Threshold: 80, ForDuration: 30},
		}},
	}
	loadCtx2, loadCancel2 := context.WithTimeout(context.Background(), alertLoadTimeout)
	longObservations := loadAlertObservations(loadCtx2, longRules, time.Now(), executeAlertQuery)
	loadCancel2()
	if longObservations.queryFailures != 0 {
		t.Fatalf("long-duration loading failures=%d", longObservations.queryFailures)
	}
	cpu, loaded := longObservations.get(7, alertItemCPU)
	if !loaded || !cpu.present || cpu.value != 95 {
		t.Fatalf("long-duration CPU observation = %#v loaded=%t", cpu, loaded)
	}
	disk, loaded := longObservations.get(7, alertItemDisk)
	if !loaded || !disk.present || disk.value != 90 || disk.mountPoint != "/" {
		t.Fatalf("long-duration disk observation = %#v loaded=%t", disk, loaded)
	}
}
