package influx

import (
	"context"
	"mosona-manager/config"
	"mosona-manager/utils"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

func LogAdd(
	teamID int64,
	userID int64,
	category string,
	message string,
	ip string,
	ua string,
	level string,
) {
	go func() {
		ipGEO, err := utils.GetIPGeoLocation(ip)
		if err != nil {
			ipGEO = utils.IPGeoResponse{
				Country:     "Unknown",
				CountryCode: "UN",
			}
		}

		point := influxdb2.NewPoint(
			"logs",
			map[string]string{
				"team_id": strconv.FormatInt(teamID, 10),
			},
			map[string]interface{}{
				"user_id":         userID,
				"category":        category,
				"message":         message,
				"ip":              ip,
				"ip_country":      ipGEO.Country,
				"ip_country_code": ipGEO.CountryCode,
				"user_agent":      ua,
				"level":           level,
			},
			time.Now(),
		)
		writeAPI := Client.WriteAPIBlocking(config.Conf.InfluxDBOrg, "logs")
		_ = writeAPI.WritePoint(context.Background(), point)
	}()
}
