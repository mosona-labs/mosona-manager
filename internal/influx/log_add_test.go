package influx

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuditLogProcessorRetriesAndDrains(t *testing.T) {
	var attempts atomic.Int32
	written := make(chan auditLogEvent, 1)
	processor := newAuditLogProcessor(4, 3, time.Second, time.Millisecond, 10*time.Millisecond, func(_ context.Context, event auditLogEvent) error {
		if attempts.Add(1) < 3 {
			return errors.New("temporary write failure")
		}
		written <- event
		return nil
	})

	event := auditLogEvent{teamID: 7, message: "changed settings", occurredAt: time.Now()}
	if !processor.enqueue(event) {
		t.Fatal("event was not enqueued")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := processor.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	select {
	case got := <-written:
		if got.teamID != event.teamID || got.message != event.message || !got.occurredAt.Equal(event.occurredAt) {
			t.Fatalf("written event = %#v, want %#v", got, event)
		}
	default:
		t.Fatal("event was not written")
	}
	stats := processor.stats()
	if stats.Enqueued != 1 || stats.Written != 1 || stats.Retried != 2 || stats.Failed != 0 || stats.Dropped != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestAuditLogProcessorBoundsQueueAndReportsDrops(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	processor := newAuditLogProcessor(1, 1, time.Second, 0, 5*time.Millisecond, func(_ context.Context, _ auditLogEvent) error {
		once.Do(func() { close(started) })
		<-release
		return nil
	})

	if !processor.enqueue(auditLogEvent{message: "in flight"}) {
		t.Fatal("first event was not enqueued")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	if !processor.enqueue(auditLogEvent{message: "queued"}) {
		t.Fatal("second event was not enqueued")
	}
	if processor.enqueue(auditLogEvent{message: "overflow"}) {
		t.Fatal("overflow event was enqueued")
	}
	if processor.enqueue(auditLogEvent{message: "high overflow", level: "high"}) {
		t.Fatal("high-priority overflow event was enqueued after its bound")
	}
	if stats := processor.stats(); stats.Enqueued != 2 || stats.Dropped != 2 {
		t.Fatalf("unexpected pre-drain stats: %#v", stats)
	}

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := processor.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if stats := processor.stats(); stats.Written != 2 || stats.Dropped != 2 {
		t.Fatalf("unexpected final stats: %#v", stats)
	}
}

func TestAuditLogProcessorShutdownCancelsBlockedWrite(t *testing.T) {
	started := make(chan struct{})
	processor := newAuditLogProcessor(1, 3, time.Minute, time.Minute, 0, func(ctx context.Context, _ auditLogEvent) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if !processor.enqueue(auditLogEvent{}) {
		t.Fatal("event was not enqueued")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("writer did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := processor.shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v, want deadline exceeded", err)
	}
	select {
	case <-processor.done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
	if stats := processor.stats(); stats.Failed != 1 {
		t.Fatalf("unexpected stats after cancellation: %#v", stats)
	}
}

func TestAuditLogProcessorRejectsEventsAfterShutdown(t *testing.T) {
	processor := newAuditLogProcessor(1, 1, time.Second, 0, 0, func(context.Context, auditLogEvent) error { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := processor.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if processor.enqueue(auditLogEvent{}) {
		t.Fatal("event was accepted after shutdown")
	}
	if stats := processor.stats(); stats.Dropped != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestAuditLogPointUsesFilterableTags(t *testing.T) {
	point := auditLogPoint(auditLogEvent{
		teamID:     42,
		userID:     7,
		category:   "server",
		level:      "high",
		message:    "updated server",
		ip:         "127.0.0.1",
		ua:         "test-agent",
		occurredAt: time.Now(),
	})

	tags := make(map[string]string)
	for _, tag := range point.TagList() {
		tags[tag.Key] = tag.Value
	}
	if tags["team_id"] != "42" || tags["category"] != "server" || tags["level"] != "high" {
		t.Fatalf("unexpected log tags: %#v", tags)
	}
	fields := make(map[string]interface{}, len(point.FieldList()))
	for _, field := range point.FieldList() {
		fields[field.Key] = field.Value
	}
	wantFields := map[string]interface{}{
		"user_id":         int64(7),
		"message":         "updated server",
		"ip":              "127.0.0.1",
		"ip_country":      "Private Network",
		"ip_country_code": "UN",
		"user_agent":      "test-agent",
	}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("log fields = %#v, want %#v", fields, wantFields)
	}
}
