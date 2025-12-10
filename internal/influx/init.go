package influx

import (
	"context"
	"log"
	"mosona-manager/internal/config"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/domain"
)

var Client influxdb2.Client

type bucketType struct {
	Name  string
	Every int64
}

func Init() {
	Client = influxdb2.NewClient(
		config.Conf.InfluxDBUrl,
		config.Conf.InfluxDBToken,
	)

	// Bucket
	buckets := []bucketType{
		{Name: "server_status_raw", Every: 1 * 24 * 60 * 60},     // 1 Days
		{Name: "server_status_minute", Every: 7 * 24 * 60 * 60},  // 7 Days
		{Name: "server_status_hourly", Every: 30 * 24 * 60 * 60}, // 30 Days
		{Name: "server_status_daily", Every: 365 * 24 * 60 * 60}, // 365 Days
		{Name: "system_usage", Every: 1 * 24 * 60 * 60},          // 1 Days
		{Name: "logs", Every: 0},                                 // No Retention
	}
	ctx := context.Background()

	organizationsAPI := Client.OrganizationsAPI()
	bucketsAPI := Client.BucketsAPI()
	org, err := organizationsAPI.FindOrganizationByName(ctx, config.Conf.InfluxDBOrg)
	if err != nil {
		log.Fatalln("Find influxdb organization error:", err)
	}

	for _, v := range buckets {
		bucket, err := bucketsAPI.FindBucketByName(ctx, v.Name)
		if err == nil && bucket != nil {
			continue
		}

		var retentionRules []domain.RetentionRule
		if v.Every > 0 {
			retentionRules = []domain.RetentionRule{{EverySeconds: v.Every}}
		}
		newBucket := &domain.Bucket{
			Name:           v.Name,
			OrgID:          org.Id,
			RetentionRules: retentionRules,
		}

		if _, err = bucketsAPI.CreateBucket(ctx, newBucket); err != nil {
			log.Printf("Failed to create bucket '%s': %v", v.Name, err)
			continue
		}
	}

	StartDownsample()
}
