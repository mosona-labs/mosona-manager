package amonitor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMonitorSnapshotSourceCoalescesConcurrentLoads(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	source := newMonitorSnapshotManager(func(context.Context, int64) (monitorSnapshot, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return monitorSnapshot{Now: 42}, nil
	}, time.Minute, time.Second).source(7)

	const workers = 32
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			result := source.get(context.Background())
			if result.err == nil && result.snapshot.Now != 42 {
				result.err = errors.New("unexpected snapshot")
			}
			errs <- result.err
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

func TestMonitorSnapshotSourceDoesNotCacheFailures(t *testing.T) {
	var calls atomic.Int32
	want := errors.New("dependency unavailable")
	source := newMonitorSnapshotManager(func(context.Context, int64) (monitorSnapshot, error) {
		if calls.Add(1) == 1 {
			return monitorSnapshot{}, want
		}
		return monitorSnapshot{Now: 42}, nil
	}, time.Minute, time.Second).source(7)

	if err := source.get(context.Background()).err; !errors.Is(err, want) {
		t.Fatalf("first get error = %v, want %v", err, want)
	}
	result := source.get(context.Background())
	if result.err != nil || result.snapshot.Now != 42 {
		t.Fatalf("second get result = %#v, error = %v", result.snapshot, result.err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("loader calls = %d, want 2", got)
	}
}

func TestMonitorSnapshotSourceFailedRefreshKeepsSuccessfulCache(t *testing.T) {
	var calls atomic.Int32
	want := errors.New("dependency unavailable")
	source := newMonitorSnapshotManager(func(context.Context, int64) (monitorSnapshot, error) {
		if calls.Add(1) == 1 {
			return monitorSnapshot{Now: 42}, nil
		}
		return monitorSnapshot{}, want
	}, time.Minute, time.Second).source(7)

	if result := source.get(context.Background()); result.err != nil || result.snapshot.Now != 42 {
		t.Fatalf("initial result = %#v, error = %v", result.snapshot, result.err)
	}
	if err := source.refresh(context.Background()).err; !errors.Is(err, want) {
		t.Fatalf("refresh error = %v, want %v", err, want)
	}
	if result := source.get(context.Background()); result.err != nil || result.snapshot.Now != 42 {
		t.Fatalf("cached result = %#v, error = %v", result.snapshot, result.err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("loader calls = %d, want 2", got)
	}
}

func TestMonitorSnapshotSourcePeriodicRefreshBypassesInitialCache(t *testing.T) {
	var calls atomic.Int32
	source := newMonitorSnapshotManager(func(context.Context, int64) (monitorSnapshot, error) {
		return monitorSnapshot{Now: int64(calls.Add(1))}, nil
	}, time.Minute, time.Second).source(7)

	if result := source.get(context.Background()); result.err != nil || result.snapshot.Now != 1 {
		t.Fatalf("initial result = %#v, error = %v", result.snapshot, result.err)
	}
	if result := source.get(context.Background()); result.err != nil || result.snapshot.Now != 1 {
		t.Fatalf("cached result = %#v, error = %v", result.snapshot, result.err)
	}
	if result := source.refresh(context.Background()); result.err != nil || result.snapshot.Now != 2 {
		t.Fatalf("refresh result = %#v, error = %v", result.snapshot, result.err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("loader calls = %d, want 2", got)
	}
}

func TestMonitorSnapshotSourceBroadcastKeepsLatestValue(t *testing.T) {
	source := &monitorSnapshotSource{
		subscribers: make(map[chan monitorSnapshotResult]struct{}),
		interval:    time.Hour,
	}
	updates, unsubscribe := source.subscribe()
	defer unsubscribe()

	source.broadcast(monitorSnapshotResult{snapshot: monitorSnapshot{Now: 1}})
	source.broadcast(monitorSnapshotResult{snapshot: monitorSnapshot{Now: 2}})
	select {
	case result := <-updates:
		if result.snapshot.Now != 2 {
			t.Fatalf("broadcast snapshot = %d, want latest value 2", result.snapshot.Now)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive broadcast")
	}
}

func TestMonitorSnapshotSourceSharesPeriodicLoadAcrossSubscribers(t *testing.T) {
	var calls atomic.Int32
	source := &monitorSnapshotSource{
		teamID:   7,
		ttl:      0,
		timeout:  time.Second,
		interval: 50 * time.Millisecond,
		loader: func(context.Context, int64) (monitorSnapshot, error) {
			calls.Add(1)
			return monitorSnapshot{Now: 42}, nil
		},
		subscribers: make(map[chan monitorSnapshotResult]struct{}),
	}
	first, unsubscribeFirst := source.subscribe()
	second, unsubscribeSecond := source.subscribe()
	defer unsubscribeFirst()
	defer unsubscribeSecond()

	for i, updates := range []<-chan monitorSnapshotResult{first, second} {
		select {
		case result := <-updates:
			if result.err != nil || result.snapshot.Now != 42 {
				t.Fatalf("subscriber %d result = %#v, error = %v", i, result.snapshot, result.err)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d did not receive periodic snapshot", i)
		}
	}
	unsubscribeFirst()
	unsubscribeSecond()
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls for two subscribers = %d, want 1", got)
	}
}

func TestMonitorSnapshotManagerSeparatesTeams(t *testing.T) {
	manager := newMonitorSnapshotManager(func(context.Context, int64) (monitorSnapshot, error) {
		return monitorSnapshot{}, nil
	}, time.Minute, time.Second)
	first := manager.source(7)
	if first != manager.source(7) {
		t.Fatal("same team did not reuse its snapshot source")
	}
	if first == manager.source(8) {
		t.Fatal("different teams unexpectedly shared a snapshot source")
	}
}

func TestMonitorSnapshotManagerEvictsSourceAfterLastSubscriber(t *testing.T) {
	manager := newMonitorSnapshotManager(func(context.Context, int64) (monitorSnapshot, error) {
		return monitorSnapshot{}, nil
	}, time.Hour, time.Second)
	first, _, unsubscribeFirst := manager.subscribe(7)
	second, _, unsubscribeSecond := manager.subscribe(7)
	if first != second {
		t.Fatal("same team did not share its snapshot source")
	}

	unsubscribeFirst()
	manager.mu.Lock()
	_, exists := manager.sources[7]
	manager.mu.Unlock()
	if !exists {
		t.Fatal("source was evicted while it still had a subscriber")
	}

	unsubscribeSecond()
	manager.mu.Lock()
	_, exists = manager.sources[7]
	manager.mu.Unlock()
	if exists {
		t.Fatal("source was not evicted after its last subscriber left")
	}
}

func TestMonitorSnapshotManagerCancelsLoadAfterLastSubscriber(t *testing.T) {
	started := make(chan struct{})
	manager := newMonitorSnapshotManager(func(ctx context.Context, _ int64) (monitorSnapshot, error) {
		close(started)
		<-ctx.Done()
		return monitorSnapshot{}, ctx.Err()
	}, time.Hour, time.Second)
	source, _, unsubscribe := manager.subscribe(7)
	result := make(chan monitorSnapshotResult, 1)
	go func() {
		result <- source.get(context.Background())
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("snapshot load did not start")
	}
	unsubscribe()
	select {
	case loaded := <-result:
		if !errors.Is(loaded.err, context.Canceled) {
			t.Fatalf("load error = %v, want %v", loaded.err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot load was not canceled")
	}
}

func TestMonitorSnapshotQuerySlotsWaitForCapacity(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	started := time.Now()
	err := acquireMonitorSnapshotQuerySlot(ctx, slots)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquire error = %v, want %v", err, context.DeadlineExceeded)
	}
	if waited := time.Since(started); waited < 10*time.Millisecond {
		t.Fatalf("slot acquisition returned without waiting: %s", waited)
	}
}
