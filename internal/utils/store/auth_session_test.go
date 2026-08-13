package store

import (
	"testing"
	"time"
)

func TestConsumeAuthSessionState(t *testing.T) {
	resetAuthSessionStateStore(t)

	state := "state-valid"
	SetAuthSessionState(state, 7, 3)

	if !ConsumeAuthSessionState(state, 7, 3, time.Now()) {
		t.Fatal("expected valid state to be consumed")
	}
	if ConsumeAuthSessionState(state, 7, 3, time.Now()) {
		t.Fatal("expected consumed state to be rejected on reuse")
	}
}

func TestConsumeAuthSessionStateRejectsMissingAndExpired(t *testing.T) {
	resetAuthSessionStateStore(t)

	if ConsumeAuthSessionState("missing", 7, 3, time.Now()) {
		t.Fatal("expected missing state to be rejected")
	}

	expired := "state-expired"
	authSessionStateStore.Lock()
	authSessionStateStore.data[expired] = authSessionState{
		createdAt:      time.Now().Add(-stateTTL - time.Second),
		providerID:     7,
		configRevision: 3,
	}
	authSessionStateStore.Unlock()

	if ConsumeAuthSessionState(expired, 7, 3, time.Now()) {
		t.Fatal("expected expired state to be rejected")
	}
	if _, ok := GetAuthSessionState(expired); ok {
		t.Fatal("expected expired state to be removed after rejection")
	}
}

func TestConsumeAuthSessionStateRejectsWrongProviderOrRevision(t *testing.T) {
	resetAuthSessionStateStore(t)

	SetAuthSessionState("wrong-provider", 7, 3)
	if ConsumeAuthSessionState("wrong-provider", 8, 3, time.Now()) {
		t.Fatal("expected provider mismatch to be rejected")
	}
	SetAuthSessionState("wrong-revision", 7, 3)
	if ConsumeAuthSessionState("wrong-revision", 7, 4, time.Now()) {
		t.Fatal("expected revision mismatch to be rejected")
	}
}

func TestRevokeAuthSessionStatesScopesByProviderAndRevision(t *testing.T) {
	resetAuthSessionStateStore(t)

	SetAuthSessionState("old", 7, 3)
	SetAuthSessionState("new", 7, 4)
	SetAuthSessionState("other-provider", 8, 3)
	RevokeAuthSessionStates(7, 3)

	if ConsumeAuthSessionState("old", 7, 3, time.Now()) {
		t.Fatal("expected matching old state to be revoked")
	}
	if !ConsumeAuthSessionState("new", 7, 4, time.Now()) {
		t.Fatal("newer state should survive a stale revocation")
	}
	if !ConsumeAuthSessionState("other-provider", 8, 3, time.Now()) {
		t.Fatal("state for another provider should survive revocation")
	}
}

func resetAuthSessionStateStore(t *testing.T) {
	t.Helper()

	authSessionStateStore.Lock()
	authSessionStateStore.data = make(map[string]authSessionState)
	authSessionStateStore.Unlock()
}
