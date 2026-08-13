package apublic

import (
	"database/sql"
	"fmt"
	"mosona-manager/internal/_type"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
)

const (
	publicRequestGlobalLimit = 32
	publicRequestIPLimit     = 8
	publicRequestWindow      = time.Second
	publicSSEIPLimit         = 64
	publicSSEHeartbeat       = 15 * time.Second
	publicSSEWriteTimeout    = 5 * time.Second
	publicSSEMaxLifetime     = 15 * time.Minute
)

var (
	publicRequests       = newPublicRequestLimiter(publicRequestGlobalLimit, publicRequestIPLimit, publicRequestWindow)
	publicSSEConnections = newPublicSSELimiter(publicSSEIPLimit)
	publicSSEErrorEvent  = []byte("event: error\ndata: {\"msg\":\"Failed to load public preview data\"}\n\n")
)

type publicRequestCount struct {
	window time.Time
	count  int
}

type publicRequestLimiter struct {
	mu          sync.Mutex
	globalLimit int
	ipLimit     int
	window      time.Duration
	global      publicRequestCount
	byIP        map[string]publicRequestCount
}

func newPublicRequestLimiter(globalLimit, ipLimit int, window time.Duration) *publicRequestLimiter {
	return &publicRequestLimiter{
		globalLimit: globalLimit,
		ipLimit:     ipLimit,
		window:      window,
		byIP:        make(map[string]publicRequestCount),
	}
}

func (l *publicRequestLimiter) allow(now time.Time, ip string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.global.window.IsZero() || now.Sub(l.global.window) >= l.window {
		l.global = publicRequestCount{window: now}
		l.byIP = make(map[string]publicRequestCount)
	}
	ipCount := l.byIP[ip]
	if ipCount.window.IsZero() {
		ipCount.window = now
	}
	if l.global.count >= l.globalLimit || ipCount.count >= l.ipLimit {
		return false
	}
	l.global.count++
	ipCount.count++
	l.byIP[ip] = ipCount
	return true
}

func limitPublicDataRequests(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if !publicRequests.allow(time.Now(), c.RealIP()) {
			setPublicPageHeaders(c)
			c.Response().Header().Set("Retry-After", "1")
			return c.JSON(http.StatusTooManyRequests, _type.H{Code: "rate_limited", Msg: "Too many public preview requests"})
		}
		return next(c)
	}
}

type publicSSELimiter struct {
	mu      sync.Mutex
	ipLimit int
	byIP    map[string]int
}

func newPublicSSELimiter(ipLimit int) *publicSSELimiter {
	return &publicSSELimiter{
		ipLimit: ipLimit,
		byIP:    make(map[string]int),
	}
}

func (l *publicSSELimiter) acquire(ip string) (func(), bool) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.byIP[ip] >= l.ipLimit {
		return nil, false
	}
	l.byIP[ip]++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.byIP[ip]--
			if l.byIP[ip] == 0 {
				delete(l.byIP, ip)
			}
		})
	}, true
}

func sse(c *echo.Context) error {
	page, ok := c.Get(publicPageContextKey).(*_type.ResolvedPublicPage)
	if !ok || page == nil {
		return publicResolveError(c, sql.ErrNoRows)
	}
	setPublicPageHeaders(c)
	release, ok := publicSSEConnections.acquire(c.RealIP())
	if !ok {
		c.Response().Header().Set("Retry-After", "10")
		return c.JSON(http.StatusTooManyRequests, _type.H{Code: "rate_limited", Msg: "Too many public preview streams"})
	}
	defer release()

	source := publicSnapshots.source(page.TeamID)
	initial := source.get(c.Request().Context())
	if initial.err != nil {
		setPublicPageHeaders(c)
		return c.JSON(http.StatusServiceUnavailable, _type.H{Code: "unavailable", Msg: "Failed to load public preview data"})
	}
	updates, unsubscribe := source.subscribe()
	defer unsubscribe()

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache, no-store")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	write := func(event []byte) error {
		controller := http.NewResponseController(c.Response())
		if err := controller.SetWriteDeadline(time.Now().Add(publicSSEWriteTimeout)); err == nil {
			defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
		}
		if _, err := c.Response().Write(event); err != nil {
			return err
		}
		return controller.Flush()
	}
	writeResult := func(result publicSnapshotResult) error {
		if result.err != nil {
			return write(publicSSEErrorEvent)
		}
		return write([]byte(fmt.Sprintf("event: update\ndata: %s\n\n", result.data)))
	}
	if err := writeResult(initial); err != nil {
		return nil
	}

	heartbeat := time.NewTicker(publicSSEHeartbeat)
	defer heartbeat.Stop()
	lifetime := time.NewTimer(publicSSEMaxLifetime)
	defer lifetime.Stop()
	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case <-lifetime.C:
			return nil
		case <-heartbeat.C:
			if err := write([]byte(": keepalive\n\n")); err != nil {
				return nil
			}
		case result := <-updates:
			if err := writeResult(result); err != nil {
				return nil
			}
		}
	}
}
