package influx

import (
	"context"
	"log"
	"mosona-manager/internal/config"
	"mosona-manager/internal/utils"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

const (
	auditLogQueueSize          = 1024
	auditLogWriteAttempts      = 3
	auditLogWriteTimeout       = 3 * time.Second
	auditLogRetryDelay         = 100 * time.Millisecond
	auditLogHighEnqueueTimeout = 250 * time.Millisecond
)

type auditLogEvent struct {
	teamID     int64
	userID     int64
	category   string
	message    string
	ip         string
	ua         string
	level      string
	occurredAt time.Time
}

// An auditLogWriter must return promptly when ctx is canceled because shutdown
// waits for the active writer before closing the InfluxDB client.
type auditLogWriter func(context.Context, auditLogEvent) error

type auditLogMetrics struct {
	enqueued atomic.Uint64
	written  atomic.Uint64
	retried  atomic.Uint64
	failed   atomic.Uint64
	dropped  atomic.Uint64
}

// AuditLogQueueStats reports process-local audit delivery counters.
type AuditLogQueueStats struct {
	Enqueued uint64
	Written  uint64
	Retried  uint64
	Failed   uint64
	Dropped  uint64
}

type auditLogProcessor struct {
	queue              chan auditLogEvent
	writer             auditLogWriter
	writeAttempts      int
	writeTimeout       time.Duration
	retryDelay         time.Duration
	highEnqueueTimeout time.Duration
	ctx                context.Context
	cancel             context.CancelFunc
	done               chan struct{}
	metrics            auditLogMetrics

	mu        sync.RWMutex
	accepting bool
}

var (
	auditLogProcessorMu sync.RWMutex
	auditLogs           *auditLogProcessor
)

func newAuditLogProcessor(
	queueSize int,
	writeAttempts int,
	writeTimeout time.Duration,
	retryDelay time.Duration,
	highEnqueueTimeout time.Duration,
	writer auditLogWriter,
) *auditLogProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	processor := &auditLogProcessor{
		queue:              make(chan auditLogEvent, queueSize),
		writer:             writer,
		writeAttempts:      writeAttempts,
		writeTimeout:       writeTimeout,
		retryDelay:         retryDelay,
		highEnqueueTimeout: highEnqueueTimeout,
		ctx:                ctx,
		cancel:             cancel,
		done:               make(chan struct{}),
		accepting:          true,
	}
	go processor.run()
	return processor
}

func startAuditLogProcessor() {
	writeAPI := Client.WriteAPIBlocking(config.Conf.InfluxDBOrg, "logs")
	processor := newAuditLogProcessor(
		auditLogQueueSize,
		auditLogWriteAttempts,
		auditLogWriteTimeout,
		auditLogRetryDelay,
		auditLogHighEnqueueTimeout,
		func(ctx context.Context, event auditLogEvent) error {
			return writeAPI.WritePoint(ctx, auditLogPoint(event))
		},
	)

	auditLogProcessorMu.Lock()
	previous := auditLogs
	auditLogs = processor
	auditLogProcessorMu.Unlock()
	if previous != nil {
		previous.stopAccepting()
		previous.cancel()
	}
}

func LogAdd(
	teamID int64,
	userID int64,
	category string,
	message string,
	ip string,
	ua string,
	level string,
) {
	event := auditLogEvent{
		teamID:     teamID,
		userID:     userID,
		category:   category,
		message:    message,
		ip:         ip,
		ua:         ua,
		level:      level,
		occurredAt: time.Now(),
	}

	auditLogProcessorMu.RLock()
	processor := auditLogs
	auditLogProcessorMu.RUnlock()
	if processor == nil {
		log.Print("Audit log dropped because the delivery worker is not running")
		return
	}
	processor.enqueue(event)
}

// ShutdownAuditLogs stops accepting events and waits for queued writes to finish.
func ShutdownAuditLogs(ctx context.Context) error {
	auditLogProcessorMu.RLock()
	processor := auditLogs
	auditLogProcessorMu.RUnlock()
	if processor == nil {
		return nil
	}
	return processor.shutdown(ctx)
}

func CurrentAuditLogQueueStats() AuditLogQueueStats {
	auditLogProcessorMu.RLock()
	processor := auditLogs
	auditLogProcessorMu.RUnlock()
	if processor == nil {
		return AuditLogQueueStats{}
	}
	return processor.stats()
}

func (p *auditLogProcessor) enqueue(event auditLogEvent) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.accepting {
		p.recordDrop("delivery worker is shutting down")
		return false
	}

	if event.level == "high" && p.highEnqueueTimeout > 0 {
		timer := time.NewTimer(p.highEnqueueTimeout)
		defer timer.Stop()
		select {
		case p.queue <- event:
			p.metrics.enqueued.Add(1)
			return true
		case <-timer.C:
			p.recordDrop("delivery queue remained full")
			return false
		case <-p.ctx.Done():
			p.recordDrop("delivery worker stopped")
			return false
		}
	}

	select {
	case p.queue <- event:
		p.metrics.enqueued.Add(1)
		return true
	default:
		p.recordDrop("delivery queue is full")
		return false
	}
}

func (p *auditLogProcessor) run() {
	defer close(p.done)
	for event := range p.queue {
		if err := p.writeWithRetry(event); err != nil {
			failed := p.metrics.failed.Add(1)
			if shouldReportCount(failed) {
				log.Printf("Audit log delivery failed after %d attempts (failed=%d): %v", p.writeAttempts, failed, err)
			}
			if p.ctx.Err() != nil {
				for range p.queue {
					p.metrics.dropped.Add(1)
				}
				return
			}
			continue
		}
		p.metrics.written.Add(1)
	}
}

func (p *auditLogProcessor) writeWithRetry(event auditLogEvent) error {
	var lastErr error
	for attempt := 1; attempt <= p.writeAttempts; attempt++ {
		if attempt > 1 {
			p.metrics.retried.Add(1)
			delay := p.retryDelay * time.Duration(1<<(attempt-2))
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-p.ctx.Done():
				timer.Stop()
				return p.ctx.Err()
			}
		}

		ctx, cancel := context.WithTimeout(p.ctx, p.writeTimeout)
		lastErr = p.writer(ctx, event)
		cancel()
		if lastErr == nil {
			return nil
		}
		if p.ctx.Err() != nil {
			return p.ctx.Err()
		}
	}
	return lastErr
}

func (p *auditLogProcessor) stopAccepting() {
	p.mu.Lock()
	if p.accepting {
		p.accepting = false
		close(p.queue)
	}
	p.mu.Unlock()
}

func (p *auditLogProcessor) shutdown(ctx context.Context) error {
	p.stopAccepting()
	select {
	case <-p.done:
		p.cancel()
		return nil
	case <-ctx.Done():
		p.cancel()
		<-p.done
		return ctx.Err()
	}
}

func (p *auditLogProcessor) stats() AuditLogQueueStats {
	return AuditLogQueueStats{
		Enqueued: p.metrics.enqueued.Load(),
		Written:  p.metrics.written.Load(),
		Retried:  p.metrics.retried.Load(),
		Failed:   p.metrics.failed.Load(),
		Dropped:  p.metrics.dropped.Load(),
	}
}

func (p *auditLogProcessor) recordDrop(reason string) {
	dropped := p.metrics.dropped.Add(1)
	if shouldReportCount(dropped) {
		log.Printf("Audit log dropped: %s (dropped=%d)", reason, dropped)
	}
}

func shouldReportCount(count uint64) bool {
	return count == 1 || count&(count-1) == 0
}

func auditLogPoint(event auditLogEvent) *write.Point {
	ipGEO, err := utils.GetIPGeoLocation(event.ip)
	if err != nil {
		ipGEO = utils.IPGeoResponse{
			Country:     "Unknown",
			CountryCode: "UN",
		}
	}

	return influxdb2.NewPoint(
		"logs",
		map[string]string{
			"team_id":  strconv.FormatInt(event.teamID, 10),
			"category": event.category,
			"level":    event.level,
		},
		map[string]interface{}{
			"user_id":         event.userID,
			"message":         event.message,
			"ip":              event.ip,
			"ip_country":      ipGEO.Country,
			"ip_country_code": ipGEO.CountryCode,
			"user_agent":      event.ua,
		},
		event.occurredAt,
	)
}
