package connection

import (
	"context"
	"sync/atomic"
	"testing"
)

func resetMainConnections(t *testing.T) {
	t.Helper()
	mu.Lock()
	old := mainConnections
	mainConnections = make(map[int64]*ManagedConn)
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		mainConnections = old
		mu.Unlock()
	})
}

func testManagedConn(closes *atomic.Int32) *ManagedConn {
	statusCtx, cancelStatus := context.WithCancel(context.Background())
	return &ManagedConn{
		statusCtx:    statusCtx,
		cancelStatus: cancelStatus,
		closeFn: func() error {
			closes.Add(1)
			return nil
		},
	}
}

func TestMainSetClosesReplacedConnection(t *testing.T) {
	resetMainConnections(t)
	var oldCloses atomic.Int32
	old := testManagedConn(&oldCloses)
	mainSetManaged(91, old)

	var newCloses atomic.Int32
	newConn := testManagedConn(&newCloses)
	mainSetManaged(91, newConn)

	if !old.Closed() || oldCloses.Load() != 1 {
		t.Fatalf("old connection closed=%v closes=%d", old.Closed(), oldCloses.Load())
	}
	if newConn.Closed() || newCloses.Load() != 0 {
		t.Fatalf("new connection closed=%v closes=%d", newConn.Closed(), newCloses.Load())
	}
}

func TestMainRemoveOnlyDeletesMatchingConnection(t *testing.T) {
	resetMainConnections(t)
	var firstCloses atomic.Int32
	first := testManagedConn(&firstCloses)
	var secondCloses atomic.Int32
	second := testManagedConn(&secondCloses)
	mu.Lock()
	mainConnections[91] = second
	mu.Unlock()

	MainRemove(91, first)
	if got, ok := MainGet(91); !ok || got != second {
		t.Fatal("old handler removed the replacement connection")
	}
	MainRemove(91, second)
	if _, ok := MainGet(91); ok {
		t.Fatal("matching connection remains registered")
	}
}

func TestOldHandlerCleanupDoesNotCloseReplacementConnection(t *testing.T) {
	resetMainConnections(t)
	var oldCloses atomic.Int32
	old := testManagedConn(&oldCloses)
	mainSetManaged(91, old)

	var replacementCloses atomic.Int32
	replacement := testManagedConn(&replacementCloses)
	mainSetManaged(91, replacement)

	MainRemove(91, old)
	old.Close()
	if got, ok := MainGet(91); !ok || got != replacement {
		t.Fatal("old handler cleanup removed the replacement connection")
	}
	if replacement.Closed() || replacementCloses.Load() != 0 {
		t.Fatal("old handler cleanup closed the replacement connection")
	}
}

func TestManagedConnCloseIsIdempotent(t *testing.T) {
	var closes atomic.Int32
	managed := testManagedConn(&closes)
	managed.Close()
	managed.Close()
	if closes.Load() != 1 {
		t.Fatalf("close calls = %d, want 1", closes.Load())
	}
}

func TestManagedConnWhileOpenRejectsWorkAfterClose(t *testing.T) {
	var closes atomic.Int32
	managed := testManagedConn(&closes)
	managed.Close()
	var ran atomic.Bool
	if managed.WhileOpen(func(context.Context) { ran.Store(true) }) {
		t.Fatal("WhileOpen reported a closed connection as open")
	}
	if ran.Load() {
		t.Fatal("work ran after connection was closed")
	}
}
