//go:build integration

package influx

import (
	"context"
	"fmt"
	"mosona-manager/internal/config"
	"testing"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

const (
	integrationInfluxURL   = "http://localhost:58086"
	integrationInfluxOrg   = "mm_org"
	integrationInfluxToken = "mm_token"
)

// TestAuditLogRoundTripIntegration verifies that a LogAdd audit event actually
// lands in a real InfluxDB 2 instance and is readable through both the raw
// query API and the GetLogsByPage read path used by the API handlers.
//
// Requires: docker run -d --name mm-review-influx -p 58086:8086 \
//   -e DOCKER_INFLUXDB_INIT_MODE=setup -e DOCKER_INFLUXDB_INIT_USERNAME=mm \
//   -e DOCKER_INFLUXDB_INIT_PASSWORD=mmpass1234 -e DOCKER_INFLUXDB_INIT_ORG=mm_org \
//   -e DOCKER_INFLUXDB_INIT_BUCKET=mm_bucket -e DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=mm_token \
//   influxdb:2-alpine
func TestAuditLogRoundTripIntegration(t *testing.T) {
	probe := influxdb2.NewClient(integrationInfluxURL, integrationInfluxToken)
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if _, err := probe.Ping(pingCtx); err != nil {
		pingCancel()
		probe.Close()
		t.Skipf("InfluxDB test container not reachable at %s: %v", integrationInfluxURL, err)
	}
	pingCancel()
	probe.Close()

	config.Conf.InfluxDBUrl = integrationInfluxURL
	config.Conf.InfluxDBOrg = integrationInfluxOrg
	config.Conf.InfluxDBToken = integrationInfluxToken

	// Real entry point: creates the client, ensures buckets, starts the audit
	// log processor and the downsamplers.
	Init()

	marker := fmt.Sprintf(`integration-marker-%d ${ 1 + 1 } "quoted" \ end`, time.Now().UnixNano())
	LogAdd(42, 7, "server", marker, "8.8.8.8", "integration-test-agent", "medium")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err := ShutdownAuditLogs(shutdownCtx); err != nil {
		shutdownCancel()
		t.Fatalf("ShutdownAuditLogs: %v", err)
	}
	shutdownCancel()

	stats := CurrentAuditLogQueueStats()
	if stats.Enqueued != 1 || stats.Written != 1 || stats.Failed != 0 || stats.Dropped != 0 {
		t.Fatalf("unexpected audit queue stats after write: %#v", stats)
	}

	// Query the point back through the raw query API.
	queryAPI := Client.QueryAPI(integrationInfluxOrg)
	queryCtx, queryCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer queryCancel()
	result, err := queryAPI.Query(queryCtx, fmt.Sprintf(`from(bucket: "logs")
	|> range(start: -1h)
	|> filter(fn: (r) => r._measurement == "logs")
	|> filter(fn: (r) => r._field == "message")
	|> filter(fn: (r) => r._value == %s)`, fluxStringLiteral(marker)))
	if err != nil {
		t.Fatalf("query logs bucket: %v", err)
	}
	defer func() { _ = result.Close() }()

	found := 0
	for result.Next() {
		record := result.Record()
		if got := record.ValueByKey("team_id"); got != "42" {
			t.Fatalf("team_id tag = %v, want 42", got)
		}
		found++
	}
	if err := result.Err(); err != nil {
		t.Fatalf("query result error: %v", err)
	}
	if found != 1 {
		t.Fatalf("found %d records for marker, want 1", found)
	}

	// Exercise the paginated read path used by the log API handlers.
	logs, total, err := GetLogsByPage(context.Background(), 42, 1, 10, "server", "medium", []int64{7}, marker)
	if err != nil {
		t.Fatalf("GetLogsByPage: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("GetLogsByPage returned total=%d logs=%d, want 1/1", total, len(logs))
	}
	entry := logs[0]
	if entry.UserID != 7 || entry.Category != "server" || entry.Message != marker ||
		entry.IP != "8.8.8.8" || entry.UserAgent != "integration-test-agent" || entry.Level != "medium" {
		t.Fatalf("unexpected log entry: %#v", entry)
	}
	if entry.IPCountry == "" || entry.IPCountryCode == "" {
		t.Fatalf("expected geo fields to be populated, got %#v", entry)
	}
}

// TestAuditLogUnreachableBackendIntegration verifies that when InfluxDB is
// unreachable, LogAdd neither blocks the caller nor hangs shutdown: the
// enqueue is non-blocking and the delivery worker gives up after bounded
// retries, recording the failure in the queue metrics.
func TestAuditLogUnreachableBackendIntegration(t *testing.T) {
	config.Conf.InfluxDBUrl = "http://127.0.0.1:59999"
	config.Conf.InfluxDBOrg = integrationInfluxOrg
	config.Conf.InfluxDBToken = integrationInfluxToken
	Client = influxdb2.NewClient(config.Conf.InfluxDBUrl, config.Conf.InfluxDBToken)
	defer Client.Close()
	startAuditLogProcessor()

	start := time.Now()
	LogAdd(42, 7, "server", "unreachable-backend-marker", "8.8.8.8", "integration-test-agent", "medium")
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("LogAdd blocked for %v with backend down", elapsed)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := ShutdownAuditLogs(shutdownCtx); err != nil {
		t.Fatalf("ShutdownAuditLogs with backend down: %v", err)
	}

	stats := CurrentAuditLogQueueStats()
	if stats.Enqueued != 1 || stats.Written != 0 || stats.Failed != 1 || stats.Dropped != 0 {
		t.Fatalf("unexpected audit queue stats with backend down: %#v", stats)
	}
	if stats.Retried != uint64(auditLogWriteAttempts-1) {
		t.Fatalf("expected %d retries, got %d", auditLogWriteAttempts-1, stats.Retried)
	}
}
