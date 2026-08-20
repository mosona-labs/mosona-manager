package influx

import (
	"context"
	"encoding/json"
	"mosona-manager/internal/_type"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

func AddServerStatusContext(ctx context.Context, serverId int64, status _type.ServerStatusType) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	serverStatusProcessorMu.RLock()
	processor := serverStatuses
	serverStatusProcessorMu.RUnlock()
	if processor == nil {
		return ErrServerStatusWriterUnavailable
	}
	occurredAt := status.Time
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	point, err := serverStatusPoint(serverId, status, occurredAt)
	if err != nil {
		return err
	}
	return processor.enqueue(ctx, point)
}

func serverStatusPoint(serverId int64, status _type.ServerStatusType, occurredAt time.Time) (*write.Point, error) {
	disksJSON, err := json.Marshal(status.Disks)
	if err != nil {
		return nil, err
	}

	point := influxdb2.NewPoint(
		"server_status",
		map[string]string{
			"server_id": strconv.FormatInt(serverId, 10),
		},
		map[string]interface{}{
			"cpu":              status.CPU,
			"mem_total_mb":     status.MemTotalMB,
			"mem_used_mb":      status.MemUsedMB,
			"swap_total_mb":    status.SwapTotalMB,
			"swap_used_mb":     status.SwapUsedMB,
			"disks":            string(disksJSON),
			"disk_read_kib_s":  status.DiskReadKibS,
			"disk_write_kib_s": status.DiskWriteKibS,
			"disk_read_iops":   status.DiskReadIOPS,
			"disk_write_iops":  status.DiskWriteIOPS,
			"rx_kib_s":         status.RxKibS,
			"tx_kib_s":         status.TxKibS,
			"rx_total_mb":      status.RxTotalMB,
			"tx_total_mb":      status.TxTotalMB,
			"tcp_total":        status.TCPTotal,
			"udp_total":        status.UDPTotal,
		},
		occurredAt,
	)
	return point, nil
}
