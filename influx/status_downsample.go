package influx

import (
	"context"
	"fmt"
	"mosona-manager/config"
	"time"
)

func StartDownsample() {
	go minuteDownsample()
	go hourlyDownsample()
	go dailyDownsample()
}

func minuteDownsample() {
	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	time.Sleep(time.Until(next))

	for {
		if err := performMinuteDownsample(); err != nil {
			//log.Printf("Failed to execute minute downsampling query: %v", err)
		}
		time.Sleep(time.Minute)
	}
}

func performMinuteDownsample() error {
	now := time.Now()
	start := now.Add(-2 * time.Minute).Truncate(time.Minute)
	stop := now.Add(-1 * time.Minute).Truncate(time.Minute)

	query := fmt.Sprintf(`from(bucket: "server_status_raw")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status")
  |> group(columns: ["server_id"])
  |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)
  |> to(bucket: "server_status_minute", org: "%s")`,
		start.Format(time.RFC3339), stop.Format(time.RFC3339), config.Conf.InfluxDBOrg)

	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)
	_, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return err
	}

	//log.Printf("Complete minute downsampling: %s to %s", start.Format("2006-01-02 15:04"), stop.Format("2006-01-02 15:04"))
	return nil
}

func hourlyDownsample() {
	now := time.Now()
	next := now.Truncate(time.Hour).Add(time.Hour)
	time.Sleep(time.Until(next))

	for {
		if err := performHourlyDownsample(); err != nil {
			//log.Printf("Failed to execute hourly downsampling query: %v", err)
		}
		time.Sleep(time.Hour)
	}
}

func performHourlyDownsample() error {
	now := time.Now()
	start := now.Add(-2 * time.Hour).Truncate(time.Hour)
	stop := now.Add(-1 * time.Hour).Truncate(time.Hour)

	query := fmt.Sprintf(`from(bucket: "server_status_raw")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status")
  |> group(columns: ["server_id"])
  |> aggregateWindow(every: 1h, fn: mean, createEmpty: false)
  |> to(bucket: "server_status_hourly", org: "%s")`,
		start.Format(time.RFC3339), stop.Format(time.RFC3339), config.Conf.InfluxDBOrg)

	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)
	_, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return err
	}

	//log.Printf("Complete hourly downsampling: %s to %s", start.Format("2006-01-02 15:04"), stop.Format("2006-01-02 15:04"))
	return nil
}

func dailyDownsample() {
	now := time.Now()
	next := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	time.Sleep(time.Until(next))

	for {
		if err := performDailyDownsample(); err != nil {
			//log.Printf("Failed to execute daily downsampling query: %v", err)
		}
		time.Sleep(24 * time.Hour)
	}
}

func performDailyDownsample() error {
	now := time.Now()
	start := now.Add(-48 * time.Hour).Truncate(24 * time.Hour)
	stop := now.Add(-24 * time.Hour).Truncate(24 * time.Hour)

	query := fmt.Sprintf(`from(bucket: "server_status_hourly")
  |> range(start: %s, stop: %s)
  |> filter(fn: (r) => r._measurement == "server_status")
  |> group(columns: ["server_id"])
  |> aggregateWindow(every: 1d, fn: mean, createEmpty: false)
  |> to(bucket: "server_status_daily", org: "%s")`,
		start.Format(time.RFC3339), stop.Format(time.RFC3339), config.Conf.InfluxDBOrg)

	queryAPI := Client.QueryAPI(config.Conf.InfluxDBOrg)
	_, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return err
	}

	//log.Printf("Complete daily downsampling: %s to %s", start.Format("2006-01-02"), stop.Format("2006-01-02"))
	return nil
}
