package influx

import (
	"context"
	"fmt"
	"mosona-manager/internal/config"
	"strings"
	"time"
)

func StartDownsample() {
	go minuteDownsample()
	go hourlyDownsample()
	go dailyDownsample()
}

func executeDownsampleQuery(query string) error {
	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)
	_, err := queryAPI.Query(context.Background(), query)
	return err
}

func minuteDownsample() {
	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	time.Sleep(time.Until(next))

	for {
		_ = performMinuteDownsample()
		time.Sleep(time.Minute)
	}
}

func downsampleFieldFilter(fields []string) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, fmt.Sprintf(`r._field == "%s"`, field))
	}
	return strings.Join(parts, " or ")
}

func buildDownsampleQuery(sourceBucket, targetBucket string, start, stop time.Time, every string) string {
	meanFields := downsampleFieldFilter([]string{
		"cpu",
		"mem_total_mb",
		"mem_used_mb",
		"swap_total_mb",
		"swap_used_mb",
		"disk_read_kib_s",
		"disk_write_kib_s",
		"disk_read_iops",
		"disk_write_iops",
		"rx_kib_s",
		"tx_kib_s",
		"rx_total_mb",
		"tx_total_mb",
	})
	lastFields := downsampleFieldFilter([]string{
		"disks",
		"disk_total_gb",
		"disk_used_gb",
		"tcp_total",
		"udp_total",
	})

	return fmt.Sprintf(`numeric = from(bucket: "%s")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status" and (%s))
  |> group(columns: ["server_id", "_measurement", "_field"])
  |> aggregateWindow(every: %s, fn: mean, createEmpty: false)
  |> to(bucket: "%s", org: "%s")

state = from(bucket: "%s")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status" and (%s))
  |> group(columns: ["server_id", "_measurement", "_field"])
  |> aggregateWindow(every: %s, fn: last, createEmpty: false)
  |> to(bucket: "%s", org: "%s")`,
		sourceBucket,
		start.Format(time.RFC3339),
		stop.Format(time.RFC3339),
		meanFields,
		every,
		targetBucket,
		config.Conf.InfluxDBOrg,
		sourceBucket,
		start.Format(time.RFC3339),
		stop.Format(time.RFC3339),
		lastFields,
		every,
		targetBucket,
		config.Conf.InfluxDBOrg,
	)
}

func performMinuteDownsample() error {
	now := time.Now()
	start := now.Add(-2 * time.Minute).Truncate(time.Minute)
	stop := now.Add(-1 * time.Minute).Truncate(time.Minute)

	query := buildDownsampleQuery("server_status_raw", "server_status_minute", start, stop, "1m")

	return executeDownsampleQuery(query)
}

func hourlyDownsample() {
	now := time.Now()
	next := now.Truncate(time.Hour).Add(time.Hour)
	time.Sleep(time.Until(next))

	for {
		_ = performHourlyDownsample()
		time.Sleep(time.Hour)
	}
}

func performHourlyDownsample() error {
	now := time.Now()
	start := now.Add(-2 * time.Hour).Truncate(time.Hour)
	stop := now.Add(-1 * time.Hour).Truncate(time.Hour)

	query := buildDownsampleQuery("server_status_raw", "server_status_hourly", start, stop, "1h")

	return executeDownsampleQuery(query)
}

func dailyDownsample() {
	now := time.Now()
	next := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	time.Sleep(time.Until(next))

	for {
		_ = performDailyDownsample()
		time.Sleep(24 * time.Hour)
	}
}

func performDailyDownsample() error {
	now := time.Now()
	start := now.Add(-48 * time.Hour).Truncate(24 * time.Hour)
	stop := now.Add(-24 * time.Hour).Truncate(24 * time.Hour)

	query := buildDownsampleQuery("server_status_hourly", "server_status_daily", start, stop, "1d")

	return executeDownsampleQuery(query)
}
