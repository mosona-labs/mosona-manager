package conn

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"mosona-manager/internal/db"
)

func setupReconcileTest(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")

	lock := lifecycleLock(91)
	lock.Lock()
	retryMu.Lock()
	mu.Lock()
	oldPool := connectPool
	oldRetries := reconcileRetries
	oldInboundStop := inboundStop
	oldRetryTimer := retryTimer
	connectPool = make(map[int64]*ServerEntry)
	reconcileRetries = make(map[int64]*reconcileRetry)
	retryTimer = time.After
	inboundStop = nil
	mu.Unlock()
	retryMu.Unlock()
	lock.Unlock()

	t.Cleanup(func() {
		lock.Lock()
		retryMu.Lock()
		mu.Lock()
		for _, entry := range connectPool {
			entry.cancel()
		}
		retries := make([]*reconcileRetry, 0, len(reconcileRetries))
		for _, retry := range reconcileRetries {
			retry.cancel()
			retries = append(retries, retry)
		}
		mu.Unlock()
		retryMu.Unlock()
		lock.Unlock()
		for _, retry := range retries {
			<-retry.done
		}

		lock.Lock()
		retryMu.Lock()
		mu.Lock()
		connectPool = oldPool
		reconcileRetries = oldRetries
		inboundStop = oldInboundStop
		retryTimer = oldRetryTimer
		mu.Unlock()
		retryMu.Unlock()
		lock.Unlock()
		db.Db = oldDB
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})
	return mock
}

func testRetry(serverID int64) *reconcileRetry {
	retryMu.Lock()
	defer retryMu.Unlock()
	return reconcileRetries[serverID]
}

func addTestOutbound(serverID int64) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(done)
	}()
	mu.Lock()
	connectPool[serverID] = &ServerEntry{cancel: cancel, done: done}
	mu.Unlock()
	return ctx
}

func setTestInboundStopper() (*[]int64, *sync.Mutex) {
	stopped := make([]int64, 0)
	var stoppedMu sync.Mutex
	RegisterInboundStopper(func(serverID int64) {
		stoppedMu.Lock()
		stopped = append(stopped, serverID)
		stoppedMu.Unlock()
	})
	return &stopped, &stoppedMu
}

func assertOutboundStopped(t *testing.T, serverID int64, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("outbound connection was not cancelled")
	}
	mu.Lock()
	_, exists := connectPool[serverID]
	mu.Unlock()
	if exists {
		t.Fatal("outbound connection remains registered")
	}
}

func TestReconcileDisabledServerStopsAllConnections(t *testing.T) {
	mock := setupReconcileTest(t)
	ctx := addTestOutbound(91)
	stopped, stoppedMu := setTestInboundStopper()
	mock.ExpectQuery(`SELECT type, allow_monitor FROM servers WHERE id = \$1`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"type", "allow_monitor"}).AddRow(1, false))

	if err := ReconcileServer(91); err != nil {
		t.Fatal(err)
	}
	assertOutboundStopped(t, 91, ctx)
	stoppedMu.Lock()
	defer stoppedMu.Unlock()
	if len(*stopped) != 1 || (*stopped)[0] != 91 {
		t.Fatalf("inbound stops = %v, want [91]", *stopped)
	}
}

func TestReconcileDeletedServerStopsAllConnections(t *testing.T) {
	mock := setupReconcileTest(t)
	ctx := addTestOutbound(91)
	stopped, stoppedMu := setTestInboundStopper()
	mock.ExpectQuery(`SELECT type, allow_monitor FROM servers WHERE id = \$1`).
		WithArgs(int64(91)).
		WillReturnError(sql.ErrNoRows)

	if err := ReconcileServer(91); err != nil {
		t.Fatal(err)
	}
	assertOutboundStopped(t, 91, ctx)
	stoppedMu.Lock()
	defer stoppedMu.Unlock()
	if len(*stopped) != 1 {
		t.Fatalf("inbound stops = %v, want one", *stopped)
	}
}

func TestReconcilePassiveServerStopsOnlyOutboundConnection(t *testing.T) {
	mock := setupReconcileTest(t)
	ctx := addTestOutbound(91)
	stopped, stoppedMu := setTestInboundStopper()
	mock.ExpectQuery(`SELECT type, allow_monitor FROM servers WHERE id = \$1`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"type", "allow_monitor"}).AddRow(2, true))

	if err := ReconcileServer(91); err != nil {
		t.Fatal(err)
	}
	assertOutboundStopped(t, 91, ctx)
	stoppedMu.Lock()
	defer stoppedMu.Unlock()
	if len(*stopped) != 0 {
		t.Fatalf("inbound stops = %v, want none", *stopped)
	}
}

func TestReconcileInvalidTypeFailsClosed(t *testing.T) {
	mock := setupReconcileTest(t)
	ctx := addTestOutbound(91)
	stopped, _ := setTestInboundStopper()
	mock.ExpectQuery(`SELECT type, allow_monitor FROM servers WHERE id = \$1`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"type", "allow_monitor"}).AddRow(9, true))

	if err := ReconcileServer(91); err == nil {
		t.Fatal("expected unsupported type error")
	}
	assertOutboundStopped(t, 91, ctx)
	if len(*stopped) != 1 {
		t.Fatalf("inbound stops = %v, want one", *stopped)
	}
}

func TestReconcileStartFailureDoesNotRetainOldConnection(t *testing.T) {
	mock := setupReconcileTest(t)
	ctx := addTestOutbound(91)
	setTestInboundStopper()
	mock.ExpectQuery(`SELECT type, allow_monitor FROM servers WHERE id = \$1`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"type", "allow_monitor"}).AddRow(1, true))
	want := errors.New("invalid agent configuration")
	mock.ExpectQuery(`SELECT agent_uid, host, port, private_key FROM servers s JOIN agents a ON s.id = a.server_id WHERE s.id = \$1`).
		WithArgs(int64(91)).
		WillReturnError(want)

	if err := ReconcileServer(91); !errors.Is(err, want) {
		t.Fatalf("ReconcileServer() error = %v, want %v", err, want)
	}
	assertOutboundStopped(t, 91, ctx)
	if retry := testRetry(91); retry == nil {
		t.Fatal("failed reconciliation did not schedule a retry")
	}
}

func TestStopServerCancelsPendingReconciliation(t *testing.T) {
	mock := setupReconcileTest(t)
	ctx := addTestOutbound(91)
	want := errors.New("database unavailable")
	mock.ExpectQuery(`SELECT type, allow_monitor FROM servers WHERE id = \$1`).
		WithArgs(int64(91)).
		WillReturnError(want)

	if err := ReconcileServer(91); !errors.Is(err, want) {
		t.Fatalf("ReconcileServer() error = %v, want %v", err, want)
	}
	retry := testRetry(91)
	if retry == nil {
		t.Fatal("retry was not registered")
	}
	assertOutboundStopped(t, 91, ctx)
	StopServer(91)
	if retry := testRetry(91); retry != nil {
		t.Fatal("retry remains registered after StopServer")
	}
}

func TestStopServerWaitsForOutboundWorker(t *testing.T) {
	setupReconcileTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	release := make(chan struct{})
	mu.Lock()
	connectPool[91] = &ServerEntry{cancel: cancel, done: done}
	mu.Unlock()
	go func() {
		<-ctx.Done()
		<-release
		close(done)
	}()

	stopped := make(chan struct{})
	go func() {
		StopServer(91)
		close(stopped)
	}()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("StopServer did not cancel the worker")
	}
	select {
	case <-stopped:
		t.Fatal("StopServer returned before the worker exited")
	default:
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("StopServer did not return after the worker exited")
	}
}

func TestReconcileRetryStopsAfterSuccess(t *testing.T) {
	mock := setupReconcileTest(t)
	timers := make(chan chan time.Time, 1)
	retryTimer = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		timers <- ch
		return ch
	}
	want := errors.New("database unavailable")
	mock.ExpectQuery(`SELECT type, allow_monitor FROM servers WHERE id = \$1`).
		WithArgs(int64(91)).
		WillReturnError(want)
	mock.ExpectQuery(`SELECT type, allow_monitor FROM servers WHERE id = \$1`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"type", "allow_monitor"}).AddRow(2, true))

	if err := ReconcileServer(91); !errors.Is(err, want) {
		t.Fatalf("ReconcileServer() error = %v, want %v", err, want)
	}
	timer := <-timers
	timer <- time.Now()

	deadline := time.After(time.Second)
	for {
		exists := testRetry(91) != nil
		if !exists {
			break
		}
		select {
		case <-deadline:
			t.Fatal("successful retry remained registered")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestReconcileActiveServerRegistersConnection(t *testing.T) {
	mock := setupReconcileTest(t)
	oldCtx := addTestOutbound(91)
	stopped, _ := setTestInboundStopper()
	mock.ExpectQuery(`SELECT type, allow_monitor FROM servers WHERE id = \$1`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"type", "allow_monitor"}).AddRow(1, true))
	mock.ExpectQuery(`SELECT agent_uid, host, port, private_key FROM servers s JOIN agents a ON s.id = a.server_id WHERE s.id = \$1`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"agent_uid", "host", "port", "private_key"}).
			AddRow("agent-uid", "127.0.0.1", 10000, "invalid-but-not-read-until-connect"))

	if err := ReconcileServer(91); err != nil {
		t.Fatal(err)
	}
	select {
	case <-oldCtx.Done():
	default:
		t.Fatal("old outbound connection was not cancelled")
	}
	mu.Lock()
	_, exists := connectPool[91]
	mu.Unlock()
	if !exists {
		t.Fatal("new outbound connection was not registered")
	}
	if len(*stopped) != 1 {
		t.Fatalf("inbound stops = %v, want one", *stopped)
	}
}

func TestRetryDelay(t *testing.T) {
	tests := []struct {
		failures int
		jitter   float64
		want     time.Duration
	}{
		{failures: 0, jitter: 0, want: time.Second},
		{failures: 1, jitter: 0, want: 2 * time.Second},
		{failures: 6, jitter: 0, want: time.Minute},
		{failures: 20, jitter: 1, want: 75 * time.Second},
	}
	for _, test := range tests {
		if got := retryDelay(test.failures, test.jitter); got != test.want {
			t.Fatalf("retryDelay(%d, %v) = %s, want %s", test.failures, test.jitter, got, test.want)
		}
	}
}
