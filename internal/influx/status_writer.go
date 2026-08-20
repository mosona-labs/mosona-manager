package influx

import (
	"context"
	"errors"
	"log"
	"mosona-manager/internal/config"
	"sync"
	"sync/atomic"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

const (
	serverStatusQueueSize     = 10_000
	serverStatusBatchSize     = 500
	serverStatusFlushInterval = time.Second
	serverStatusWriteTimeout  = 5 * time.Second
	serverStatusRetryDelay    = 200 * time.Millisecond
	serverStatusWriteAttempts = 2
)

var (
	ErrServerStatusWriterUnavailable = errors.New("server status writer is unavailable")
	ErrServerStatusWriterStopped     = errors.New("server status writer is stopped")

	serverStatusProcessorMu sync.RWMutex
	serverStatuses          *serverStatusProcessor
)

// A serverStatusBatchWriter must return promptly when ctx is canceled because
// shutdown waits for the active writer before closing the InfluxDB client.
type serverStatusBatchWriter func(context.Context, []*write.Point) error

type serverStatusMetrics struct {
	enqueued      atomic.Uint64
	written       atomic.Uint64
	retried       atomic.Uint64
	failed        atomic.Uint64
	dropped       atomic.Uint64
	dropEvents    atomic.Uint64
	failureEvents atomic.Uint64
}

// ServerStatusQueueStats reports process-local status delivery counters.
type ServerStatusQueueStats struct {
	Enqueued uint64 // Points accepted by the processor.
	Written  uint64 // Points acknowledged by InfluxDB.
	Retried  uint64 // Batch retry attempts.
	Failed   uint64 // Points discarded after all write attempts failed.
	Dropped  uint64 // Points evicted by overflow or shutdown cancellation.
	Pending  uint64 // Points queued, being assembled, or currently being written.
}

type serverStatusProcessor struct {
	queue         []*write.Point
	head          int
	size          int
	inFlight      int
	batchSize     int
	flushInterval time.Duration
	writeTimeout  time.Duration
	retryDelay    time.Duration
	writeAttempts int
	writer        serverStatusBatchWriter
	wake          chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	metrics       serverStatusMetrics

	mu        sync.Mutex
	accepting bool
}

func newServerStatusProcessor(
	queueSize int,
	batchSize int,
	flushInterval time.Duration,
	writeTimeout time.Duration,
	retryDelay time.Duration,
	writeAttempts int,
	writer serverStatusBatchWriter,
) *serverStatusProcessor {
	if queueSize <= 0 || batchSize <= 0 || flushInterval <= 0 || writeTimeout <= 0 || writeAttempts <= 0 || writer == nil {
		panic("invalid server status processor configuration")
	}
	ctx, cancel := context.WithCancel(context.Background())
	processor := &serverStatusProcessor{
		queue:         make([]*write.Point, queueSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		writeTimeout:  writeTimeout,
		retryDelay:    retryDelay,
		writeAttempts: writeAttempts,
		writer:        writer,
		wake:          make(chan struct{}, 1),
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
		accepting:     true,
	}
	go processor.run()
	return processor
}

func startServerStatusProcessor() {
	writeAPI := Client.WriteAPIBlocking(config.Conf.InfluxDBOrg, "server_status_raw")
	processor := newServerStatusProcessor(
		serverStatusQueueSize,
		serverStatusBatchSize,
		serverStatusFlushInterval,
		serverStatusWriteTimeout,
		serverStatusRetryDelay,
		serverStatusWriteAttempts,
		func(ctx context.Context, points []*write.Point) error {
			return writeAPI.WritePoint(ctx, points...)
		},
	)

	serverStatusProcessorMu.Lock()
	previous := serverStatuses
	serverStatuses = processor
	serverStatusProcessorMu.Unlock()
	if previous != nil {
		previous.abort()
		<-previous.done
	}
}

// ShutdownServerStatuses stops accepting samples and drains queued writes.
func ShutdownServerStatuses(ctx context.Context) error {
	serverStatusProcessorMu.RLock()
	processor := serverStatuses
	serverStatusProcessorMu.RUnlock()
	if processor == nil {
		return nil
	}
	return processor.shutdown(ctx)
}

func CurrentServerStatusQueueStats() ServerStatusQueueStats {
	serverStatusProcessorMu.RLock()
	processor := serverStatuses
	serverStatusProcessorMu.RUnlock()
	if processor == nil {
		return ServerStatusQueueStats{}
	}
	return processor.stats()
}

func (p *serverStatusProcessor) enqueue(ctx context.Context, point *write.Point) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p.mu.Lock()
	if !p.accepting {
		p.mu.Unlock()
		return ErrServerStatusWriterStopped
	}
	dropped := p.pushLocked(point)
	p.metrics.enqueued.Add(1)
	p.mu.Unlock()
	if dropped {
		p.recordDrop(1, "queue is full")
	}
	p.notify()
	return nil
}

func (p *serverStatusProcessor) pushLocked(point *write.Point) bool {
	dropped := p.size == len(p.queue)
	if dropped {
		p.queue[p.head] = nil
		p.head = (p.head + 1) % len(p.queue)
		p.size--
	}
	tail := (p.head + p.size) % len(p.queue)
	p.queue[tail] = point
	p.size++
	return dropped
}

func (p *serverStatusProcessor) take(max int) ([]*write.Point, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := min(max, p.size)
	points := make([]*write.Point, count)
	for i := range count {
		points[i] = p.queue[p.head]
		p.queue[p.head] = nil
		p.head = (p.head + 1) % len(p.queue)
		p.size--
	}
	p.inFlight += count
	return points, p.accepting
}

func (p *serverStatusProcessor) run() {
	defer close(p.done)
	for {
		batch, accepting := p.take(p.batchSize)
		if len(batch) == 0 {
			if !accepting {
				return
			}
			select {
			case <-p.wake:
				continue
			case <-p.ctx.Done():
				p.dropRemaining("delivery worker stopped")
				return
			}
		}

		if len(batch) < p.batchSize && accepting {
			var canceled bool
			batch, canceled = p.fillBatch(batch)
			if canceled {
				p.dropRemaining("delivery worker stopped")
				return
			}
		}

		switch p.writeBatch(batch) {
		case serverStatusBatchWritten, serverStatusBatchFailed:
			p.completeBatch(len(batch))
			continue
		case serverStatusBatchCanceled:
			p.dropRemaining("delivery worker stopped")
			return
		}
	}
}

func (p *serverStatusProcessor) fillBatch(batch []*write.Point) ([]*write.Point, bool) {
	timer := time.NewTimer(p.flushInterval)
	defer timer.Stop()
	for len(batch) < p.batchSize {
		more, accepting := p.take(p.batchSize - len(batch))
		batch = append(batch, more...)
		if len(batch) == p.batchSize || !accepting {
			return batch, false
		}
		select {
		case <-p.wake:
		case <-timer.C:
			return batch, false
		case <-p.ctx.Done():
			return batch, true
		}
	}
	return batch, false
}

type serverStatusBatchResult uint8

const (
	serverStatusBatchWritten serverStatusBatchResult = iota
	serverStatusBatchFailed
	serverStatusBatchCanceled
)

func (p *serverStatusProcessor) writeBatch(points []*write.Point) serverStatusBatchResult {
	var lastErr error
	for attempt := 1; attempt <= p.writeAttempts; attempt++ {
		if attempt > 1 {
			p.metrics.retried.Add(1)
			timer := time.NewTimer(p.retryDelay)
			select {
			case <-timer.C:
			case <-p.ctx.Done():
				timer.Stop()
				return serverStatusBatchCanceled
			}
		}

		ctx, cancel := context.WithTimeout(p.ctx, p.writeTimeout)
		lastErr = p.writer(ctx, points)
		cancel()
		if lastErr == nil {
			p.metrics.written.Add(uint64(len(points)))
			return serverStatusBatchWritten
		}
		if p.ctx.Err() != nil {
			return serverStatusBatchCanceled
		}
	}

	p.metrics.failed.Add(uint64(len(points)))
	failures := p.metrics.failureEvents.Add(1)
	if shouldReportCount(failures) {
		log.Printf(
			"Server status batch failed after %d attempts (points=%d failed=%d): %v",
			p.writeAttempts, len(points), p.metrics.failed.Load(), lastErr,
		)
	}
	return serverStatusBatchFailed
}

func (p *serverStatusProcessor) stopAccepting() {
	p.mu.Lock()
	p.accepting = false
	p.mu.Unlock()
	p.notify()
}

func (p *serverStatusProcessor) abort() {
	p.stopAccepting()
	p.cancel()
	p.notify()
}

func (p *serverStatusProcessor) shutdown(ctx context.Context) error {
	p.stopAccepting()
	select {
	case <-p.done:
		p.cancel()
		return nil
	case <-ctx.Done():
		p.cancel()
		p.notify()
		<-p.done
		return ctx.Err()
	}
}

func (p *serverStatusProcessor) notify() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *serverStatusProcessor) completeBatch(points int) {
	p.mu.Lock()
	p.inFlight -= points
	p.mu.Unlock()
}

func (p *serverStatusProcessor) dropRemaining(reason string) {
	p.mu.Lock()
	inFlight := p.inFlight
	p.inFlight = 0
	queued := p.size
	for p.size > 0 {
		p.queue[p.head] = nil
		p.head = (p.head + 1) % len(p.queue)
		p.size--
	}
	p.mu.Unlock()
	p.recordDrop(uint64(inFlight+queued), reason)
}

func (p *serverStatusProcessor) recordDrop(points uint64, reason string) {
	if points == 0 {
		return
	}
	p.metrics.dropped.Add(points)
	events := p.metrics.dropEvents.Add(1)
	if shouldReportCount(events) {
		log.Printf("Server status points dropped: %s (points=%d dropped=%d)", reason, points, p.metrics.dropped.Load())
	}
}

func (p *serverStatusProcessor) stats() ServerStatusQueueStats {
	p.mu.Lock()
	pending := p.size + p.inFlight
	p.mu.Unlock()
	return ServerStatusQueueStats{
		Enqueued: p.metrics.enqueued.Load(),
		Written:  p.metrics.written.Load(),
		Retried:  p.metrics.retried.Load(),
		Failed:   p.metrics.failed.Load(),
		Dropped:  p.metrics.dropped.Load(),
		Pending:  uint64(pending),
	}
}
