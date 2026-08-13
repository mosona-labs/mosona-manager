package app

import (
	"context"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/redis"
	"net/http"
	"sort"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v5"
)

const (
	readinessTimeout         = 2 * time.Second
	healthCheckClientTimeout = 2500 * time.Millisecond
)

var initializationComplete atomic.Bool

type dependencyProbe struct {
	name  string
	check func(context.Context) error
}

type probeResult struct {
	name string
	err  error
}

func registerHealthRoutes(e *echo.Echo) {
	probes := defaultDependencyProbes()
	handler := readinessHandler(initializationComplete.Load, probes, readinessTimeout)

	// Keep /health as the backward-compatible liveness endpoint.
	e.GET("/health", livenessHandler)
	e.GET("/health/live", livenessHandler)
	e.GET("/health/ready", handler)
}

func livenessHandler(c *echo.Context) error {
	return c.JSON(http.StatusOK, _type.H{
		Code: "ok",
		Msg:  "Service is live",
	})
}

func readinessHandler(initialized func() bool, probes []dependencyProbe, timeout time.Duration) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if !initialized() {
			return readinessResponse(c, []string{"initialization"})
		}

		ctx, cancel := context.WithTimeout(c.Request().Context(), timeout)
		defer cancel()

		results := make(chan probeResult, len(probes))
		pending := make(map[string]struct{}, len(probes))
		for _, probe := range probes {
			pending[probe.name] = struct{}{}
			go func(probe dependencyProbe) {
				results <- probeResult{name: probe.name, err: probe.check(ctx)}
			}(probe)
		}

		failed := make([]string, 0, len(probes))
		for len(pending) > 0 {
			select {
			case result := <-results:
				if _, ok := pending[result.name]; !ok {
					continue
				}
				delete(pending, result.name)
				if result.err != nil {
					failed = append(failed, result.name)
				}
			case <-ctx.Done():
				for name := range pending {
					failed = append(failed, name)
				}
				pending = nil
			}
		}

		return readinessResponse(c, failed)
	}
}

func readinessResponse(c *echo.Context, failed []string) error {
	if len(failed) == 0 {
		return c.JSON(http.StatusOK, _type.H{
			Code: "ok",
			Msg:  "Service is ready",
		})
	}

	sort.Strings(failed)
	return c.JSON(http.StatusServiceUnavailable, _type.H{
		Code: "not_ready",
		Msg:  "Service is not ready",
		Data: failed,
	})
}

func defaultDependencyProbes() []dependencyProbe {
	return []dependencyProbe{
		{
			name: "postgres",
			check: func(ctx context.Context) error {
				if db.Db == nil {
					return errors.New("postgres client is not initialized")
				}
				return db.Db.PingContext(ctx)
			},
		},
		{
			name: "redis",
			check: func(ctx context.Context) error {
				if redis.Client == nil {
					return errors.New("redis client is not initialized")
				}
				return redis.Client.Ping(ctx).Err()
			},
		},
		{
			name: "influxdb",
			check: func(ctx context.Context) error {
				if influx.Client == nil {
					return errors.New("influxdb client is not initialized")
				}
				org, err := influx.Client.OrganizationsAPI().FindOrganizationByName(ctx, config.Conf.InfluxDBOrg)
				if err != nil {
					return err
				}
				if org == nil {
					return errors.New("influxdb organization is unavailable")
				}
				return nil
			},
		},
	}
}
