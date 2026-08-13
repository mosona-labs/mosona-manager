package apublic

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublicSnapshotManagerCoalescesConcurrentLoads(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	manager := newPublicSnapshotManager(func(context.Context, int64) (publicSnapshot, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return publicSnapshot{Now: 42}, nil
	}, time.Minute, time.Second)

	const workers = 32
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			snapshot, err := manager.get(context.Background(), 7)
			if err == nil && snapshot.Now != 42 {
				err = errors.New("unexpected snapshot")
			}
			errs <- err
		}()
	}
	<-started
	close(release)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
}

func TestPublicSnapshotManagerCachesFailuresDuringTTL(t *testing.T) {
	var calls atomic.Int32
	want := errors.New("dependency unavailable")
	manager := newPublicSnapshotManager(func(context.Context, int64) (publicSnapshot, error) {
		calls.Add(1)
		return publicSnapshot{}, want
	}, time.Minute, time.Second)

	for range 10 {
		if _, err := manager.get(context.Background(), 7); !errors.Is(err, want) {
			t.Fatalf("get error = %v, want %v", err, want)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
}

func TestPublicSnapshotSourceBroadcastKeepsLatestValue(t *testing.T) {
	source := &publicSnapshotSource{
		subscribers: make(map[chan publicSnapshotResult]struct{}),
		interval:    time.Hour,
	}
	updates, unsubscribe := source.subscribe()
	defer unsubscribe()

	source.broadcast(publicSnapshotResult{snapshot: publicSnapshot{Now: 1}})
	source.broadcast(publicSnapshotResult{snapshot: publicSnapshot{Now: 2}})
	select {
	case result := <-updates:
		if result.snapshot.Now != 2 {
			t.Fatalf("broadcast snapshot = %d, want latest value 2", result.snapshot.Now)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive broadcast")
	}
}

func TestPublicSnapshotSourceSharesPeriodicLoadAcrossSubscribers(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	source := &publicSnapshotSource{
		teamID:   7,
		ttl:      0,
		timeout:  time.Second,
		interval: 20 * time.Millisecond,
		loader: func(context.Context, int64) (publicSnapshot, error) {
			if calls.Add(1) == 1 {
				close(started)
			}
			<-release
			return publicSnapshot{Now: 42}, nil
		},
		subscribers: make(map[chan publicSnapshotResult]struct{}),
	}
	first, unsubscribeFirst := source.subscribe()
	defer unsubscribeFirst()
	second, unsubscribeSecond := source.subscribe()
	defer unsubscribeSecond()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("periodic loader did not start")
	}
	close(release)
	for i, updates := range []<-chan publicSnapshotResult{first, second} {
		select {
		case result := <-updates:
			if result.err != nil || result.snapshot.Now != 42 {
				t.Fatalf("subscriber %d result = %#v, error = %v", i, result.snapshot, result.err)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d did not receive periodic snapshot", i)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls for two subscribers = %d, want 1", got)
	}
}
