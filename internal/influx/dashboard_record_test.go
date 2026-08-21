package influx

import (
	"context"
	"encoding/json"
	"errors"
	"mosona-manager/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

const countCSVResult = `#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,long,string,string
#group,false,false,true,true,false,true,true
#default,_result,,,,,,
,result,table,_start,_stop,_value,_field,_measurement
,,0,2025-01-01T00:00:00Z,2026-01-01T00:00:00Z,2,cpu,server_status

`

var countBuckets = []string{
	"server_status_raw",
	"server_status_minute",
	"server_status_hourly",
	"server_status_daily",
}

func countBucketFromRequest(request *http.Request) (string, error) {
	var payload struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		return "", err
	}
	for _, bucket := range countBuckets {
		if strings.Contains(payload.Query, `from(bucket: "`+bucket+`")`) {
			return bucket, nil
		}
	}
	return "", errors.New("count query did not name a known bucket")
}

func useInfluxTestClient(t *testing.T, serverURL string) {
	t.Helper()
	previousClient := Client
	previousOrg := config.Conf.InfluxDBOrg
	testClient := influxdb2.NewClient(serverURL, "test-token")
	Client = testClient
	config.Conf.InfluxDBOrg = "test-org"
	t.Cleanup(func() {
		testClient.Close()
		Client = previousClient
		config.Conf.InfluxDBOrg = previousOrg
	})
}

func TestGetAllBucketAllServerRecordCountContextRunsQueriesConcurrently(t *testing.T) {
	started := make(chan struct{}, len(countBuckets))
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseQueries := func() { releaseOnce.Do(func() { close(release) }) }
	seen := make(map[string]int, len(countBuckets))
	var seenMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		bucket, err := countBucketFromRequest(request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		seenMu.Lock()
		seen[bucket]++
		seenMu.Unlock()
		started <- struct{}{}
		<-release
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(countCSVResult))
	}))
	defer server.Close()
	defer releaseQueries()
	useInfluxTestClient(t, server.URL)

	type countResult struct {
		count int64
		err   error
	}
	done := make(chan countResult, 1)
	go func() {
		count, err := GetAllBucketAllServerRecordCountContext(context.Background())
		done <- countResult{count: count, err: err}
	}()

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for requestCount := 1; requestCount <= len(countBuckets); requestCount++ {
		select {
		case <-started:
		case <-timer.C:
			t.Fatalf("only %d of %d bucket queries started concurrently", requestCount-1, len(countBuckets))
		}
	}
	releaseQueries()

	result := <-done
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.count != 8 {
		t.Fatalf("record count = %d, want 8", result.count)
	}
	seenMu.Lock()
	defer seenMu.Unlock()
	if len(seen) != len(countBuckets) {
		t.Fatalf("queried buckets = %#v, want all four status buckets", seen)
	}
	for _, bucket := range countBuckets {
		if seen[bucket] != 1 {
			t.Fatalf("bucket %q query count = %d, want 1", bucket, seen[bucket])
		}
	}
}

func TestGetAllBucketAllServerRecordCountContextIncludesFailedBucket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		bucket, err := countBucketFromRequest(request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if bucket == "server_status_hourly" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"internal error","message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(countCSVResult))
	}))
	defer server.Close()
	useInfluxTestClient(t, server.URL)

	_, err := GetAllBucketAllServerRecordCountContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "count records in server_status_hourly") {
		t.Fatalf("error = %v, want failed bucket name", err)
	}
}

func TestGetAllBucketAllServerRecordCountContextHonorsCancellation(t *testing.T) {
	started := make(chan struct{}, len(countBuckets))
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseQueries := func() { releaseOnce.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		started <- struct{}{}
		<-release
	}))
	defer server.Close()
	defer releaseQueries()
	useInfluxTestClient(t, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := GetAllBucketAllServerRecordCountContext(ctx)
		done <- err
	}()

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for requestCount := 1; requestCount <= len(countBuckets); requestCount++ {
		select {
		case <-started:
		case <-timer.C:
			t.Fatalf("only %d of %d bucket queries started", requestCount-1, len(countBuckets))
		}
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("query error = %v, want context.Canceled", err)
		}
		releaseQueries()
	case <-time.After(2 * time.Second):
		t.Fatal("record count queries ignored context cancellation")
	}
}

func TestGetSystemUsageContextHonorsCancellation(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseQuery := func() { releaseOnce.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		started <- struct{}{}
		<-release
	}))
	defer server.Close()
	defer releaseQuery()
	useInfluxTestClient(t, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := GetSystemUsageContext(ctx)
		done <- err
	}()

	select {
	case <-started:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("system usage query did not start")
	}

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("query error = %v, want context.Canceled", err)
		}
		releaseQuery()
	case <-time.After(2 * time.Second):
		t.Fatal("system usage query ignored context cancellation")
	}
}
