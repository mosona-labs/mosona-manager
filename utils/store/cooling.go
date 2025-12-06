package store

import (
	"sync"
	"time"
)

var (
	coolingStore = struct {
		sync.RWMutex
		data map[int64]int64
	}{data: make(map[int64]int64)}
)

const cleanupIntervalCooling = 60 // 1 minute

func init() {
	go cleanupExpiredCoolingLock()
}

func GetCooling(uid int64) (int64, bool) {
	coolingStore.RLock()
	defer coolingStore.RUnlock()
	val, ok := coolingStore.data[uid]
	return val, ok
}

func SetCooling(uid int64, expire int64) {
	coolingStore.Lock()
	defer coolingStore.Unlock()
	coolingStore.data[uid] = expire
}

func cleanupExpiredCoolingLock() {
	ticker := time.NewTicker(cleanupIntervalCooling * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().Unix()

		coolingStore.Lock()
		for uid, expire := range coolingStore.data {
			if now > expire {
				delete(coolingStore.data, uid)
			}
		}
		coolingStore.Unlock()
	}
}
