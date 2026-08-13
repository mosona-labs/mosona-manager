package notification

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	maxConcurrentDeliveries = 8
	maxDeliveriesPerMinute  = 30
	maxGlobalPerMinute      = 300
	deliveryTimeout         = 15 * time.Second
)

var (
	ErrRateLimited = errors.New("notification delivery rate limit exceeded")
	deliverySlots  = make(chan struct{}, maxConcurrentDeliveries)
	deliveryRates  = newTargetRateLimiter(maxDeliveriesPerMinute, time.Minute)
	globalRates    = newTargetRateLimiter(maxGlobalPerMinute, time.Minute)
	hostedSend     = sendHosted
)

func Send(ctx context.Context, target, message string) error {
	now := time.Now()
	if !globalRates.Allow("global", now) || !deliveryRates.Allow(target, now) {
		return ErrRateLimited
	}

	deliveryCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	defer cancel()
	select {
	case deliverySlots <- struct{}{}:
	case <-deliveryCtx.Done():
		return deliveryCtx.Err()
	}
	if err := ValidateTarget(deliveryCtx, target); err != nil {
		<-deliverySlots
		return err
	}

	parsed, _ := url.ParseRequestURI(target)
	if parsed != nil && isGenericScheme(parsed.Scheme) {
		defer func() { <-deliverySlots }()
		return sendGeneric(deliveryCtx, target, message)
	}

	result := make(chan error, 1)
	go func() {
		defer func() { <-deliverySlots }()
		result <- hostedSend(deliveryCtx, target, message)
	}()
	select {
	case err := <-result:
		if err != nil {
			return errors.New("notification delivery failed")
		}
		return nil
	case <-deliveryCtx.Done():
		return deliveryCtx.Err()
	}
}

func isGenericScheme(scheme string) bool {
	return strings.EqualFold(scheme, "generic") ||
		strings.EqualFold(scheme, "generic+http") ||
		strings.EqualFold(scheme, "generic+https")
}

type targetRateLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	counters map[[sha256.Size]byte]targetCounter
	calls    uint64
}

type targetCounter struct {
	started time.Time
	count   int
}

func newTargetRateLimiter(limit int, window time.Duration) *targetRateLimiter {
	return &targetRateLimiter{limit: limit, window: window, counters: make(map[[sha256.Size]byte]targetCounter)}
}

func (l *targetRateLimiter) Allow(target string, now time.Time) bool {
	key := sha256.Sum256([]byte(target))
	l.mu.Lock()
	defer l.mu.Unlock()

	l.calls++
	if l.calls%256 == 0 {
		for item, counter := range l.counters {
			if now.Sub(counter.started) >= 2*l.window {
				delete(l.counters, item)
			}
		}
	}
	counter := l.counters[key]
	if counter.started.IsZero() || now.Sub(counter.started) >= l.window {
		l.counters[key] = targetCounter{started: now, count: 1}
		return true
	}
	if counter.count >= l.limit {
		return false
	}
	counter.count++
	l.counters[key] = counter
	return true
}
