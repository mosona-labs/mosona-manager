package influx

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

func testServerStatusPoint(id string) *write.Point {
	return influxdb2.NewPoint(
		"server_status",
		map[string]string{"server_id": id},
		map[string]interface{}{"cpu": 1.0},
		time.Unix(1, 0),
	)
}

func TestServerStatusProcessorWritesFullBatch(t *testing.T) {
	written := make(chan []*write.Point, 1)
	processor := newServerStatusProcessor(
		10, 3, time.Hour, time.Second, time.Millisecond, 2,
		func(_ context.Context, points []*write.Point) error {
			select {
			case written <- append([]*write.Point(nil), points...):
				return nil
			default:
				return errors.New("unexpected additional batch")
			}
		},
	)
	for i := range 3 {
		if err := processor.enqueue(context.Background(), testServerStatusPoint(string(rune('1'+i)))); err != nil {
			t.Fatalf("enqueue point %d: %v", i, err)
		}
	}

	select {
	case batch := <-written:
		if len(batch) != 3 {
			t.Fatalf("batch size = %d, want 3", len(batch))
		}
	case <-time.After(time.Second):
		t.Fatal("full batch was not written")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := processor.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if stats := processor.stats(); stats.Enqueued != 3 || stats.Written != 3 || stats.Pending != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestServerStatusProcessorFlushesPartialBatch(t *testing.T) {
	written := make(chan int, 1)
	processor := newServerStatusProcessor(
		10, 5, 20*time.Millisecond, time.Second, time.Millisecond, 2,
		func(_ context.Context, points []*write.Point) error {
			select {
			case written <- len(points):
				return nil
			default:
				return errors.New("unexpected additional batch")
			}
		},
	)
	for i := range 2 {
		if err := processor.enqueue(context.Background(), testServerStatusPoint(string(rune('1'+i)))); err != nil {
			t.Fatalf("enqueue point %d: %v", i, err)
		}
	}

	select {
	case size := <-written:
		if size != 2 {
			t.Fatalf("batch size = %d, want 2", size)
		}
	case <-time.After(time.Second):
		t.Fatal("partial batch was not flushed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := processor.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestServerStatusProcessorDropsOldestWithoutBlockingEnqueue(t *testing.T) {
	first := testServerStatusPoint("1")
	oldestQueued := testServerStatusPoint("2")
	third := testServerStatusPoint("3")
	newest := testServerStatusPoint("4")
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var written []*write.Point
	processor := newServerStatusProcessor(
		2, 1, time.Hour, time.Second, time.Millisecond, 1,
		func(_ context.Context, points []*write.Point) error {
			if points[0] == first {
				once.Do(func() { close(started) })
				<-release
			}
			mu.Lock()
			written = append(written, points...)
			mu.Unlock()
			return nil
		},
	)
	if err := processor.enqueue(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("writer did not start")
	}
	for _, point := range []*write.Point{oldestQueued, third, newest} {
		if err := processor.enqueue(context.Background(), point); err != nil {
			t.Fatalf("enqueue while writer blocked: %v", err)
		}
	}
	if stats := processor.stats(); stats.Enqueued != 4 || stats.Dropped != 1 || stats.Pending != 3 {
		t.Fatalf("unexpected stats while blocked: %#v", stats)
	}

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := processor.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(written) != 3 || written[0] != first || written[1] != third || written[2] != newest {
		t.Fatalf("written points = %#v, want first, third, newest", written)
	}
}

func TestServerStatusProcessorRetriesOnce(t *testing.T) {
	var attempts atomic.Int32
	processor := newServerStatusProcessor(
		10, 2, time.Hour, time.Second, time.Millisecond, 2,
		func(_ context.Context, _ []*write.Point) error {
			if attempts.Add(1) == 1 {
				return errors.New("temporary failure")
			}
			return nil
		},
	)
	for i := range 2 {
		if err := processor.enqueue(context.Background(), testServerStatusPoint(string(rune('1'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := processor.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("write attempts = %d, want 2", got)
	}
	if stats := processor.stats(); stats.Written != 2 || stats.Retried != 1 || stats.Failed != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestServerStatusProcessorRecordsFinalBatchFailure(t *testing.T) {
	processor := newServerStatusProcessor(
		10, 2, time.Hour, time.Second, time.Millisecond, 2,
		func(context.Context, []*write.Point) error {
			return errors.New("persistent failure")
		},
	)
	for i := range 2 {
		if err := processor.enqueue(context.Background(), testServerStatusPoint(string(rune('1'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := processor.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if stats := processor.stats(); stats.Retried != 1 || stats.Failed != 2 || stats.Written != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestServerStatusProcessorShutdownCancelsBlockedWrite(t *testing.T) {
	started := make(chan struct{})
	processor := newServerStatusProcessor(
		1, 1, time.Hour, time.Minute, time.Minute, 2,
		func(ctx context.Context, _ []*write.Point) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	)
	if err := processor.enqueue(context.Background(), testServerStatusPoint("1")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("writer did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := processor.shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v, want %v", err, context.DeadlineExceeded)
	}
	if stats := processor.stats(); stats.Dropped != 1 || stats.Pending != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	if err := processor.enqueue(context.Background(), testServerStatusPoint("2")); !errors.Is(err, ErrServerStatusWriterStopped) {
		t.Fatalf("enqueue after shutdown error = %v, want %v", err, ErrServerStatusWriterStopped)
	}
}

func TestServerStatusProcessorAcceptsConcurrentProducers(t *testing.T) {
	var written atomic.Uint64
	processor := newServerStatusProcessor(
		4_000, 50, time.Hour, time.Second, time.Millisecond, 2,
		func(_ context.Context, points []*write.Point) error {
			written.Add(uint64(len(points)))
			return nil
		},
	)

	const producers = 32
	const pointsPerProducer = 100
	var wait sync.WaitGroup
	errs := make(chan error, producers)
	wait.Add(producers)
	for producer := range producers {
		go func() {
			defer wait.Done()
			for point := range pointsPerProducer {
				id := strconv.Itoa(producer*pointsPerProducer + point)
				if err := processor.enqueue(context.Background(), testServerStatusPoint(id)); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := processor.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	want := uint64(producers * pointsPerProducer)
	if got := written.Load(); got != want {
		t.Fatalf("written points = %d, want %d", got, want)
	}
	if stats := processor.stats(); stats.Enqueued != want || stats.Written != want || stats.Dropped != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}
