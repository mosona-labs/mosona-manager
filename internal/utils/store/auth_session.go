package store

import (
	"sync"
	"time"
)

var (
	authSessionStateStore = struct {
		sync.RWMutex
		data map[string]authSessionState
	}{data: make(map[string]authSessionState)}
)

type authSessionState struct {
	createdAt      time.Time
	providerID     int
	configRevision int64
}

const (
	stateTTL        = 10 * time.Minute
	cleanupInterval = 10 * time.Minute
)

func init() {
	go cleanupExpiredData()
}

func GetAuthSessionState(state string) (time.Time, bool) {
	authSessionStateStore.RLock()
	defer authSessionStateStore.RUnlock()
	val, ok := authSessionStateStore.data[state]
	return val.createdAt, ok
}

func SetAuthSessionState(state string, providerID int, configRevision int64) {
	authSessionStateStore.Lock()
	defer authSessionStateStore.Unlock()
	authSessionStateStore.data[state] = authSessionState{
		createdAt:      time.Now(),
		providerID:     providerID,
		configRevision: configRevision,
	}
}

func ConsumeAuthSessionState(state string, providerID int, configRevision int64, now time.Time) bool {
	authSessionStateStore.Lock()
	defer authSessionStateStore.Unlock()

	entry, ok := authSessionStateStore.data[state]
	if !ok {
		return false
	}
	delete(authSessionStateStore.data, state)

	return entry.providerID == providerID &&
		entry.configRevision == configRevision &&
		now.Sub(entry.createdAt) <= stateTTL
}

func DeleteAuthSessionState(state string) {
	authSessionStateStore.Lock()
	defer authSessionStateStore.Unlock()
	delete(authSessionStateStore.data, state)
}

func RevokeAuthSessionStates(providerID int, throughConfigRevision int64) {
	authSessionStateStore.Lock()
	defer authSessionStateStore.Unlock()
	for state, entry := range authSessionStateStore.data {
		if entry.providerID == providerID && entry.configRevision <= throughConfigRevision {
			delete(authSessionStateStore.data, state)
		}
	}
}

func cleanupExpiredData() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		authSessionStateStore.Lock()
		for state, entry := range authSessionStateStore.data {
			if now.Sub(entry.createdAt) > stateTTL {
				delete(authSessionStateStore.data, state)
			}
		}
		authSessionStateStore.Unlock()
	}
}
