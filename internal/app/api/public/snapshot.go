package apublic

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
	publicSnapshotInterval = 5 * time.Second
	publicSnapshotTimeout  = 8 * time.Second
)

type publicSnapshot struct {
	Servers    []_type.PublicMonitor             `json:"servers"`
	Categories []_type.Category                  `json:"categories"`
	Status     map[int64]*_type.ServerStatusType `json:"status"`
	Now        int64                             `json:"now"`
}

type publicSnapshotResult struct {
	snapshot publicSnapshot
	data     []byte
	err      error
}

type publicSnapshotLoad struct {
	done   chan struct{}
	result publicSnapshotResult
}

type publicSnapshotSource struct {
	teamID   int64
	loader   func(context.Context, int64) (publicSnapshot, error)
	ttl      time.Duration
	timeout  time.Duration
	interval time.Duration

	mu          sync.Mutex
	cached      publicSnapshotResult
	loadedAt    time.Time
	load        *publicSnapshotLoad
	subscribers map[chan publicSnapshotResult]struct{}
	loopStop    chan struct{}
	loopRunning bool
}

type publicSnapshotManager struct {
	mu       sync.Mutex
	sources  map[int64]*publicSnapshotSource
	loader   func(context.Context, int64) (publicSnapshot, error)
	ttl      time.Duration
	timeout  time.Duration
	interval time.Duration
}

var (
	publicSnapshots          = newPublicSnapshotManager(loadPublicSnapshot, publicSnapshotInterval, publicSnapshotTimeout)
	publicSnapshotQuerySlots = make(chan struct{}, 8)
)

func newPublicSnapshotManager(
	loader func(context.Context, int64) (publicSnapshot, error),
	interval, timeout time.Duration,
) *publicSnapshotManager {
	return &publicSnapshotManager{
		sources:  make(map[int64]*publicSnapshotSource),
		loader:   loader,
		ttl:      interval,
		timeout:  timeout,
		interval: interval,
	}
}

func (m *publicSnapshotManager) source(teamID int64) *publicSnapshotSource {
	m.mu.Lock()
	defer m.mu.Unlock()
	if source := m.sources[teamID]; source != nil {
		return source
	}
	source := &publicSnapshotSource{
		teamID:      teamID,
		loader:      m.loader,
		ttl:         m.ttl,
		timeout:     m.timeout,
		interval:    m.interval,
		subscribers: make(map[chan publicSnapshotResult]struct{}),
	}
	m.sources[teamID] = source
	return source
}

func (m *publicSnapshotManager) get(ctx context.Context, teamID int64) (publicSnapshot, error) {
	result := m.source(teamID).get(ctx)
	return result.snapshot, result.err
}

func (s *publicSnapshotSource) get(ctx context.Context) publicSnapshotResult {
	s.mu.Lock()
	if !s.loadedAt.IsZero() && time.Since(s.loadedAt) < s.ttl {
		result := s.cached
		s.mu.Unlock()
		return result
	}
	if load := s.load; load != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return publicSnapshotResult{err: ctx.Err()}
		case <-load.done:
			return load.result
		}
	}
	load := &publicSnapshotLoad{done: make(chan struct{})}
	s.load = load
	s.mu.Unlock()

	queryCtx, cancel := context.WithTimeout(context.Background(), s.timeout)
	snapshot, err := s.loader(queryCtx, s.teamID)
	cancel()
	result := publicSnapshotResult{snapshot: snapshot, err: err}
	if err == nil {
		result.data, err = json.Marshal(snapshot)
		result.err = err
	}

	s.mu.Lock()
	s.cached = result
	s.loadedAt = time.Now()
	load.result = result
	s.load = nil
	close(load.done)
	s.mu.Unlock()
	return result
}

func (s *publicSnapshotSource) subscribe() (<-chan publicSnapshotResult, func()) {
	updates := make(chan publicSnapshotResult, 1)
	s.mu.Lock()
	s.subscribers[updates] = struct{}{}
	if !s.loopRunning {
		stop := make(chan struct{})
		s.loopStop = stop
		s.loopRunning = true
		go s.run(stop)
	}
	s.mu.Unlock()

	var once sync.Once
	return updates, func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.subscribers, updates)
			if len(s.subscribers) == 0 && s.loopStop != nil {
				close(s.loopStop)
				s.loopStop = nil
			}
			s.mu.Unlock()
		})
	}
}

func (s *publicSnapshotSource) run(stop <-chan struct{}) {
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
			s.broadcast(s.get(context.Background()))
		}
	}
}

func (s *publicSnapshotSource) broadcast(result publicSnapshotResult) {
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

func loadPublicSnapshot(ctx context.Context, teamID int64) (publicSnapshot, error) {
	select {
	case publicSnapshotQuerySlots <- struct{}{}:
		defer func() { <-publicSnapshotQuerySlots }()
	case <-ctx.Done():
		return publicSnapshot{}, ctx.Err()
	default:
		return publicSnapshot{}, errors.New("public snapshot loader is busy")
	}
	servers, err := db.ListPublicMonitoredServersContext(ctx, teamID)
	if err != nil {
		return publicSnapshot{}, err
	}
	categories, err := db.GetCategoriesByTeamContext(ctx, teamID)
	if err != nil {
		return publicSnapshot{}, err
	}
	ids := make([]int64, 0, len(servers))
	for _, server := range servers {
		ids = append(ids, server.ID)
	}
	statusMap, err := influx.GetLatestServerStatusBatchContext(ctx, ids)
	if err != nil {
		return publicSnapshot{}, err
	}
	if statusMap == nil {
		return publicSnapshot{}, errors.New("public status query returned no result")
	}
	return publicSnapshot{
		Servers: servers, Categories: categories, Status: statusMap, Now: time.Now().Unix(),
	}, nil
}
