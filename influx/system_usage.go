package influx

import (
	"context"
	"mosona-manager/config"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

func UsageRecordAdd(cpu int, memory int) {
	point := influxdb2.NewPoint(
		"system_usage",
		map[string]string{},
		map[string]interface{}{
			"cpu_usage": cpu,
			"memory":    memory,
		},
		time.Now(),
	)
	writeAPI := Client.WriteAPIBlocking(config.Conf.InfluxDBOrg, "system_usage")
	_ = writeAPI.WritePoint(context.Background(), point)
}
