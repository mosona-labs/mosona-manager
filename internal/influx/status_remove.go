package influx

import (
	"context"
	"fmt"
	"mosona-manager/internal/config"
	"time"
)

func RemoveServerStatus(serverId int64) error {
	deleteAPI := Client.DeleteAPI()

	start := time.Unix(0, 0)
	end := time.Now()

	predicate := fmt.Sprintf(`server_id="%d"`, serverId)

	ctx := context.Background()
	buckets := []string{
		"server_status_raw",
		"server_status_minute",
		"server_status_hourly",
		"server_status_daily",
	}

	var errs []error
	for _, bucket := range buckets {
		if err := deleteAPI.DeleteWithName(
			ctx,
			config.Conf.InfluxDBOrg,
			bucket,
			start, end, predicate,
		); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete from %s: %w", bucket, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during deletion: %v", errs)
	}

	return nil
}
