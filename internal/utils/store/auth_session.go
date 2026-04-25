package store

import (
	"sync"
	"time"
)

var (
	authSessionStateStore = struct {
		sync.RWMutex
		data map[string]time.Time
	}{data: make(map[string]time.Time)}
)

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
	return val, ok
}

func SetAuthSessionState(state string) {
	authSessionStateStore.Lock()
	defer authSessionStateStore.Unlock()
	authSessionStateStore.data[state] = time.Now()
}

func ConsumeAuthSessionState(state string, now time.Time) bool {
	authSessionStateStore.Lock()
	defer authSessionStateStore.Unlock()

	createdAt, ok := authSessionStateStore.data[state]
	if !ok {
		return false
	}
	delete(authSessionStateStore.data, state)

	return now.Sub(createdAt) <= stateTTL
}

func DeleteAuthSessionState(state string) {
	authSessionStateStore.Lock()
	defer authSessionStateStore.Unlock()
	delete(authSessionStateStore.data, state)
}

func cleanupExpiredData() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		authSessionStateStore.Lock()
		for state, createdAt := range authSessionStateStore.data {
			if now.Sub(createdAt) > stateTTL {
				delete(authSessionStateStore.data, state)
			}
		}
		authSessionStateStore.Unlock()
	}
}
