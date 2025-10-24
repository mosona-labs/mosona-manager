package influx

import (
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"mosona-manager/config"
)

var Client influxdb2.Client

func Init() {
	Client = influxdb2.NewClient(
		config.Conf.InfluxDBUrl,
		config.Conf.InfluxDBToken,
	)
}
