package influx

import (
	"context"
	"errors"
	"mosona-manager/internal/_type"
	"reflect"
	"testing"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

func TestServerStatusPointUsesReceptionTimestamp(t *testing.T) {
	occurredAt := time.Unix(123, 456)
	point, err := serverStatusPoint(7, _type.ServerStatusType{CPU: 42}, occurredAt)
	if err != nil {
		t.Fatalf("build point: %v", err)
	}
	if !point.Time().Equal(occurredAt) {
		t.Fatalf("point time = %v, want %v", point.Time(), occurredAt)
	}
	if tags := point.TagList(); len(tags) != 1 || tags[0].Key != "server_id" || tags[0].Value != "7" {
		t.Fatalf("point tags = %#v", tags)
	}
}

func TestAddServerStatusContextReturnsUnavailableWithoutProcessor(t *testing.T) {
	serverStatusProcessorMu.Lock()
	previous := serverStatuses
	serverStatuses = nil
	serverStatusProcessorMu.Unlock()
	defer func() {
		serverStatusProcessorMu.Lock()
		serverStatuses = previous
		serverStatusProcessorMu.Unlock()
	}()

	err := AddServerStatusContext(context.Background(), 7, _type.ServerStatusType{})
	if !errors.Is(err, ErrServerStatusWriterUnavailable) {
		t.Fatalf("add status error = %v, want %v", err, ErrServerStatusWriterUnavailable)
	}
}

func TestAddServerStatusContextHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := AddServerStatusContext(ctx, 7, _type.ServerStatusType{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("add status error = %v, want %v", err, context.Canceled)
	}
}

func TestAddServerStatusContextEnqueuesMappedPoint(t *testing.T) {
	written := make(chan []*write.Point, 1)
	processor := newServerStatusProcessor(
		2, 1, time.Hour, time.Second, time.Millisecond, 1,
		func(_ context.Context, points []*write.Point) error {
			written <- append([]*write.Point(nil), points...)
			return nil
		},
	)
	serverStatusProcessorMu.Lock()
	previous := serverStatuses
	serverStatuses = processor
	serverStatusProcessorMu.Unlock()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = processor.shutdown(ctx)
		serverStatusProcessorMu.Lock()
		serverStatuses = previous
		serverStatusProcessorMu.Unlock()
	}()

	occurredAt := time.Unix(123, 456)
	status := _type.ServerStatusType{
		CPU:           1,
		MemTotalMB:    2,
		MemUsedMB:     3,
		SwapTotalMB:   4,
		SwapUsedMB:    5,
		Disks:         []_type.DiskInfo{{MountPoint: "/", TotalGB: 6, UsedGB: 7}},
		DiskReadKibS:  8,
		DiskWriteKibS: 9,
		DiskReadIOPS:  10,
		DiskWriteIOPS: 11,
		RxKibS:        12,
		TxKibS:        13,
		RxTotalMB:     14,
		TxTotalMB:     15,
		TCPTotal:      16,
		UDPTotal:      17,
		Time:          occurredAt,
	}
	if err := AddServerStatusContext(context.Background(), 7, status); err != nil {
		t.Fatalf("add status: %v", err)
	}

	var point *write.Point
	select {
	case points := <-written:
		if len(points) != 1 {
			t.Fatalf("written points = %d, want 1", len(points))
		}
		point = points[0]
	case <-time.After(time.Second):
		t.Fatal("status point was not written")
	}
	if !point.Time().Equal(occurredAt) {
		t.Fatalf("point time = %v, want %v", point.Time(), occurredAt)
	}
	if tags := point.TagList(); len(tags) != 1 || tags[0].Key != "server_id" || tags[0].Value != "7" {
		t.Fatalf("point tags = %#v", tags)
	}
	fields := make(map[string]interface{}, len(point.FieldList()))
	for _, field := range point.FieldList() {
		fields[field.Key] = field.Value
	}
	wantFields := map[string]interface{}{
		"cpu":              float64(1),
		"mem_total_mb":     float64(2),
		"mem_used_mb":      float64(3),
		"swap_total_mb":    float64(4),
		"swap_used_mb":     float64(5),
		"disks":            `[{"mp":"/","total_gb":6,"used_gb":7}]`,
		"disk_read_kib_s":  float64(8),
		"disk_write_kib_s": float64(9),
		"disk_read_iops":   float64(10),
		"disk_write_iops":  float64(11),
		"rx_kib_s":         float64(12),
		"tx_kib_s":         float64(13),
		"rx_total_mb":      float64(14),
		"tx_total_mb":      float64(15),
		"tcp_total":        int64(16),
		"udp_total":        int64(17),
	}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("point fields = %#v, want %#v", fields, wantFields)
	}
}
