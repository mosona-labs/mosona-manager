package amonitor

import (
	"context"
	"encoding/json"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"sync"
	"time"
)

const (
	monitorSnapshotInterval = 3 * time.Second
	monitorSnapshotTimeout  = 8 * time.Second
)

type monitorSnapshot struct {
	Servers []_type.MonitorList               `json:"servers"`
	Status  map[int64]*_type.ServerStatusType `json:"status"`
	Now     int64                             `json:"now"`
}

type monitorSnapshotResult struct {
	snapshot monitorSnapshot
	data     []byte
	err      error
}

type monitorSnapshotLoad struct {
	done   chan struct{}
	result monitorSnapshotResult
}

type monitorSnapshotSource struct {
	teamID   int64
	loader   func(context.Context, int64) (monitorSnapshot, error)
	ttl      time.Duration
	timeout  time.Duration
	interval time.Duration

	mu          sync.Mutex
	cached      monitorSnapshotResult
	loadedAt    time.Time
	load        *monitorSnapshotLoad
	loadCancel  context.CancelFunc
	subscribers map[chan monitorSnapshotResult]struct{}
	loopStop    chan struct{}
	loopRunning bool
}

type monitorSnapshotManager struct {
	mu       sync.Mutex
	sources  map[int64]*monitorSnapshotSource
	loader   func(context.Context, int64) (monitorSnapshot, error)
	ttl      time.Duration
	timeout  time.Duration
	interval time.Duration
}

var (
	monitorSnapshots          = newMonitorSnapshotManager(loadMonitorSnapshot, monitorSnapshotInterval, monitorSnapshotTimeout)
	monitorSnapshotQuerySlots = make(chan struct{}, 64)
)

func newMonitorSnapshotManager(
	loader func(context.Context, int64) (monitorSnapshot, error),
	interval, timeout time.Duration,
) *monitorSnapshotManager {
	return &monitorSnapshotManager{
		sources:  make(map[int64]*monitorSnapshotSource),
		loader:   loader,
		ttl:      interval,
		timeout:  timeout,
		interval: interval,
	}
}

func (m *monitorSnapshotManager) source(teamID int64) *monitorSnapshotSource {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sourceLocked(teamID)
}

func (m *monitorSnapshotManager) sourceLocked(teamID int64) *monitorSnapshotSource {
	if source := m.sources[teamID]; source != nil {
		return source
	}
	source := &monitorSnapshotSource{
		teamID:      teamID,
		loader:      m.loader,
		ttl:         m.ttl,
		timeout:     m.timeout,
		interval:    m.interval,
		subscribers: make(map[chan monitorSnapshotResult]struct{}),
	}
	m.sources[teamID] = source
	return source
}

func (m *monitorSnapshotManager) subscribe(teamID int64) (
	*monitorSnapshotSource,
	<-chan monitorSnapshotResult,
	func(),
) {
	m.mu.Lock()
	source := m.sourceLocked(teamID)
	source.mu.Lock()
	updates := source.subscribeLocked()
	source.mu.Unlock()
	m.mu.Unlock()

	var once sync.Once
	return source, updates, func() {
		once.Do(func() {
			m.mu.Lock()
			source.mu.Lock()
			idle := source.unsubscribeLocked(updates)
			if idle && m.sources[teamID] == source {
				delete(m.sources, teamID)
			}
			source.mu.Unlock()
			m.mu.Unlock()
		})
	}
}

func (s *monitorSnapshotSource) get(ctx context.Context) monitorSnapshotResult {
	return s.loadResult(ctx, true)
}

func (s *monitorSnapshotSource) refresh(ctx context.Context) monitorSnapshotResult {
	return s.loadResult(ctx, false)
}

func (s *monitorSnapshotSource) loadResult(ctx context.Context, useCache bool) monitorSnapshotResult {
	s.mu.Lock()
	if useCache && !s.loadedAt.IsZero() && time.Since(s.loadedAt) < s.ttl {
		result := s.cached
		s.mu.Unlock()
		return result
	}
	if load := s.load; load != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return monitorSnapshotResult{err: ctx.Err()}
		case <-load.done:
			return load.result
		}
	}
	load := &monitorSnapshotLoad{done: make(chan struct{})}
	s.load = load
	queryCtx, cancel := context.WithTimeout(context.Background(), s.timeout)
	s.loadCancel = cancel
	s.mu.Unlock()

	snapshot, err := s.loader(queryCtx, s.teamID)
	cancel()
	result := monitorSnapshotResult{snapshot: snapshot, err: err}
	if err == nil {
		result.data, err = json.Marshal(snapshot)
		result.err = err
	}

	s.mu.Lock()
	if result.err == nil {
		s.cached = result
		s.loadedAt = time.Now()
	}
	load.result = result
	s.load = nil
	s.loadCancel = nil
	close(load.done)
	s.mu.Unlock()
	return result
}

func (s *monitorSnapshotSource) subscribe() (<-chan monitorSnapshotResult, func()) {
	s.mu.Lock()
	updates := s.subscribeLocked()
	s.mu.Unlock()

	var once sync.Once
	return updates, func() {
		once.Do(func() {
			s.mu.Lock()
			s.unsubscribeLocked(updates)
			s.mu.Unlock()
		})
	}
}

func (s *monitorSnapshotSource) subscribeLocked() chan monitorSnapshotResult {
	updates := make(chan monitorSnapshotResult, 1)
	s.subscribers[updates] = struct{}{}
	if !s.loopRunning {
		stop := make(chan struct{})
		s.loopStop = stop
		s.loopRunning = true
		go s.run(stop)
	}
	return updates
}

func (s *monitorSnapshotSource) unsubscribeLocked(updates chan monitorSnapshotResult) bool {
	delete(s.subscribers, updates)
	if len(s.subscribers) != 0 {
		return false
	}
	if s.loopStop != nil {
		close(s.loopStop)
		s.loopStop = nil
	}
	if s.loadCancel != nil {
		s.loadCancel()
	}
	return true
}

func (s *monitorSnapshotSource) run(stop <-chan struct{}) {
	ticker := time.NewTicker(s.interval)
	defer func() {
		ticker.Stop()
		s.mu.Lock()
		s.loopRunning = false
		if len(s.subscribers) > 0 {
			nextStop := make(chan struct{})
			s.loopStop = nextStop
			s.loopRunning = true
			go s.run(nextStop)
		}
		s.mu.Unlock()
	}()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			result := s.refresh(context.Background())
			select {
			case <-stop:
				return
			default:
				s.broadcast(result)
			}
		}
	}
}

func (s *monitorSnapshotSource) broadcast(result monitorSnapshotResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for subscriber := range s.subscribers {
		select {
		case subscriber <- result:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- result:
			default:
			}
		}
	}
}

func loadMonitorSnapshot(ctx context.Context, teamID int64) (monitorSnapshot, error) {
	if err := acquireMonitorSnapshotQuerySlot(ctx, monitorSnapshotQuerySlots); err != nil {
		return monitorSnapshot{}, err
	}
	defer func() { <-monitorSnapshotQuerySlots }()

	servers, err := db.ListMonitoredServersContext(ctx, teamID)
	if err != nil {
		return monitorSnapshot{}, err
	}
	ids := make([]int64, 0, len(servers))
	for _, server := range servers {
		ids = append(ids, server.ID)
	}
	statusMap, err := influx.GetLatestServerStatusBatchContext(ctx, ids)
	if err != nil {
		return monitorSnapshot{}, err
	}
	if statusMap == nil {
		return monitorSnapshot{}, errors.New("monitor status query returned no result")
	}
	return monitorSnapshot{Servers: servers, Status: statusMap, Now: time.Now().Unix()}, nil
}

func acquireMonitorSnapshotQuerySlot(ctx context.Context, slots chan struct{}) error {
	select {
	case slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
