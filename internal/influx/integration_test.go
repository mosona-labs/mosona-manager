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
// query API and the cursor-based GetLogs read path used by the API handlers.
//
//	Requires: docker run -d --name mm-review-influx -p 58086:8086 \
//	  -e DOCKER_INFLUXDB_INIT_MODE=setup -e DOCKER_INFLUXDB_INIT_USERNAME=mm \
//	  -e DOCKER_INFLUXDB_INIT_PASSWORD=mmpass1234 -e DOCKER_INFLUXDB_INIT_ORG=mm_org \
//	  -e DOCKER_INFLUXDB_INIT_BUCKET=mm_bucket -e DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=mm_token \
//	  influxdb:2-alpine
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
		if got := record.ValueByKey("category"); got != "server" {
			t.Fatalf("category tag = %v, want server", got)
		}
		if got := record.ValueByKey("level"); got != "medium" {
			t.Fatalf("level tag = %v, want medium", got)
		}
		found++
	}
	if err := result.Err(); err != nil {
		t.Fatalf("query result error: %v", err)
	}
	if found != 1 {
		t.Fatalf("found %d records for marker, want 1", found)
	}

	// Exercise the cursor-based read path used by the log API handlers.
	page, err := GetLogs(context.Background(), 42, 10, "server", "medium", []int64{7}, marker, time.Now().Add(-time.Hour), time.Now().Add(time.Minute), "")
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if page.HasMore || page.NextCursor != "" || len(page.Logs) != 1 {
		t.Fatalf("GetLogs returned page=%#v, want one final record", page)
	}
	entry := page.Logs[0]
	if entry.UserID != 7 || entry.Category != "server" || entry.Message != marker ||
		entry.IP != "8.8.8.8" || entry.UserAgent != "integration-test-agent" || entry.Level != "medium" {
		t.Fatalf("unexpected log entry: %#v", entry)
	}
	if entry.IPCountry == "" || entry.IPCountryCode == "" {
		t.Fatalf("expected geo fields to be populated, got %#v", entry)
	}

	t.Run("legacy field schema remains readable", func(t *testing.T) {
		legacyMarker := fmt.Sprintf("legacy-integration-marker-%d", time.Now().UnixNano())
		occurredAt := time.Now().UTC()
		legacyPoint := influxdb2.NewPoint(
			"logs",
			map[string]string{"team_id": "42"},
			map[string]interface{}{
				"user_id":         int64(11),
				"category":        "security",
				"message":         legacyMarker,
				"ip":              "127.0.0.2",
				"ip_country":      "Private Network",
				"ip_country_code": "UN",
				"user_agent":      "legacy-agent",
				"level":           "high",
			},
			occurredAt,
		)
		writeCtx, writeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer writeCancel()
		if err := Client.WriteAPIBlocking(integrationInfluxOrg, "logs").WritePoint(writeCtx, legacyPoint); err != nil {
			t.Fatalf("write legacy audit log: %v", err)
		}

		legacyPage, err := GetLogs(
			context.Background(), 42, 10, "security", "high", []int64{11}, legacyMarker,
			occurredAt.Add(-time.Minute), occurredAt.Add(time.Minute), "",
		)
		if err != nil {
			t.Fatalf("GetLogs legacy audit log: %v", err)
		}
		if legacyPage.HasMore || legacyPage.NextCursor != "" || len(legacyPage.Logs) != 1 {
			t.Fatalf("GetLogs returned legacy page=%#v, want one final record", legacyPage)
		}
		legacy := legacyPage.Logs[0]
		if legacy.UserID != 11 || legacy.Category != "security" || legacy.Message != legacyMarker ||
			legacy.IP != "127.0.0.2" || legacy.IPCountry != "Private Network" || legacy.IPCountryCode != "UN" ||
			legacy.UserAgent != "legacy-agent" || legacy.Level != "high" {
			t.Fatalf("unexpected legacy log entry: %#v", legacy)
		}
	})
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
