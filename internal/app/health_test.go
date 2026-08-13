package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

func TestLivenessDoesNotDependOnReadiness(t *testing.T) {
	e := echo.New()
	e.GET("/health/live", livenessHandler)

	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"code":"ok"`) {
		t.Fatalf("liveness response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestRegisterHealthRoutesSeparatesLivenessAndReadiness(t *testing.T) {
	initializationComplete.Store(false)
	t.Cleanup(func() { initializationComplete.Store(false) })

	e := echo.New()
	registerHealthRoutes(e)
	for _, path := range []string{"/health", "/health/live"} {
		recorder := httptest.NewRecorder()
		e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want %d", path, recorder.Code, http.StatusOK)
		}
	}

	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /health/ready status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestReadinessRequiresInitialization(t *testing.T) {
	called := false
	handler := readinessHandler(func() bool { return false }, []dependencyProbe{{
		name: "postgres",
		check: func(context.Context) error {
			called = true
			return nil
		},
	}}, time.Second)

	recorder := serveHealthHandler(t, handler)
	if called {
		t.Fatal("dependency probes ran before initialization completed")
	}
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"initialization"`) {
		t.Fatalf("readiness response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestReadinessReportsFailedDependenciesWithoutInternalErrors(t *testing.T) {
	handler := readinessHandler(func() bool { return true }, []dependencyProbe{
		{name: "postgres", check: func(context.Context) error { return errors.New("password=secret host=db.internal") }},
		{name: "redis", check: func(context.Context) error { return nil }},
		{name: "influxdb", check: func(context.Context) error { return errors.New("token=secret") }},
	}, time.Second)

	recorder := serveHealthHandler(t, handler)
	body := recorder.Body.String()
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(body, `"data":["influxdb","postgres"]`) {
		t.Fatalf("readiness response does not contain sorted failed dependencies: %s", body)
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "db.internal") {
		t.Fatalf("readiness response leaked dependency error details: %s", body)
	}
}

func TestReadinessRunsProbesConcurrentlyAndHonorsOverallTimeout(t *testing.T) {
	timeout := 50 * time.Millisecond
	blocked := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	handler := readinessHandler(func() bool { return true }, []dependencyProbe{
		{name: "postgres", check: blocked},
		{name: "redis", check: blocked},
		{name: "influxdb", check: blocked},
	}, timeout)

	started := time.Now()
	recorder := serveHealthHandler(t, handler)
	elapsed := time.Since(started)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if elapsed > 4*timeout {
		t.Fatalf("readiness took %s; probes may not be running concurrently", elapsed)
	}
	for _, name := range []string{"influxdb", "postgres", "redis"} {
		if !strings.Contains(recorder.Body.String(), `"`+name+`"`) {
			t.Fatalf("timed-out dependency %q missing from response: %s", name, recorder.Body.String())
		}
	}
}

func TestReadinessReturnsOKWhenAllDependenciesPass(t *testing.T) {
	handler := readinessHandler(func() bool { return true }, []dependencyProbe{
		{name: "postgres", check: func(context.Context) error { return nil }},
		{name: "redis", check: func(context.Context) error { return nil }},
		{name: "influxdb", check: func(context.Context) error { return nil }},
	}, time.Second)

	recorder := serveHealthHandler(t, handler)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"code":"ok"`) {
		t.Fatalf("readiness response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestHealthCheckURLUsesReadinessAndLoopbackForWildcardBinds(t *testing.T) {
	tests := map[string]string{
		"0.0.0.0": "http://127.0.0.1:8080/health/ready",
		"::":      "http://127.0.0.1:8080/health/ready",
		"::1":     "http://[::1]:8080/health/ready",
		"hub":     "http://hub:8080/health/ready",
	}
	for host, want := range tests {
		if got := healthCheckURL(host, 8080); got != want {
			t.Errorf("healthCheckURL(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestHealthCheckUsesReadinessStatus(t *testing.T) {
	status := http.StatusServiceUnavailable
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/health/ready" {
			t.Errorf("health check path = %q, want /health/ready", r.URL.Path)
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader("{}")),
			Header:     make(http.Header),
		}, nil
	})}

	if err := runHealthCheck(client, "http://127.0.0.1:8080/health/ready"); err == nil {
		t.Fatal("HealthCheck succeeded for a not-ready response")
	}
	status = http.StatusOK
	if err := runHealthCheck(client, "http://127.0.0.1:8080/health/ready"); err != nil {
		t.Fatalf("HealthCheck failed for a ready response: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func serveHealthHandler(t *testing.T, handler echo.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.GET("/health/ready", handler)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	return recorder
}
