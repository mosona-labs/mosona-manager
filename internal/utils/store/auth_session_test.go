package store

import (
	"testing"
	"time"
)

func TestConsumeAuthSessionState(t *testing.T) {
	resetAuthSessionStateStore(t)

	state := "state-valid"
	SetAuthSessionState(state)

	if !ConsumeAuthSessionState(state, time.Now()) {
		t.Fatal("expected valid state to be consumed")
	}
	if ConsumeAuthSessionState(state, time.Now()) {
		t.Fatal("expected consumed state to be rejected on reuse")
	}
}

func TestConsumeAuthSessionStateRejectsMissingAndExpired(t *testing.T) {
	resetAuthSessionStateStore(t)

	if ConsumeAuthSessionState("missing", time.Now()) {
		t.Fatal("expected missing state to be rejected")
	}

	expired := "state-expired"
	authSessionStateStore.Lock()
	authSessionStateStore.data[expired] = time.Now().Add(-stateTTL - time.Second)
	authSessionStateStore.Unlock()

	if ConsumeAuthSessionState(expired, time.Now()) {
		t.Fatal("expected expired state to be rejected")
	}
	if _, ok := GetAuthSessionState(expired); ok {
		t.Fatal("expected expired state to be removed after rejection")
	}
}

func resetAuthSessionStateStore(t *testing.T) {
	t.Helper()

	authSessionStateStore.Lock()
	authSessionStateStore.data = make(map[string]time.Time)
	authSessionStateStore.Unlock()
}
