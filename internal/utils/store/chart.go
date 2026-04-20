package store

import (
	"sync"
	"time"
)

type ChartCacheEntry struct {
	Data   []byte
	Expire int64
}

var (
	chartCacheStore = struct {
		sync.RWMutex
		data map[string]ChartCacheEntry
	}{data: make(map[string]ChartCacheEntry)}
)

const cleanupIntervalChart = 30 * time.Second

func init() {
	go cleanupExpiredChartCache()
}

func GetChartCache(key string) ([]byte, bool) {
	now := time.Now().Unix()

	chartCacheStore.RLock()
	entry, ok := chartCacheStore.data[key]
	chartCacheStore.RUnlock()
	if !ok || now > entry.Expire {
		if ok {
			chartCacheStore.Lock()
			delete(chartCacheStore.data, key)
			chartCacheStore.Unlock()
		}
		return nil, false
	}

	return entry.Data, true
}

func SetChartCache(key string, data []byte, ttl time.Duration) {
	chartCacheStore.Lock()
	defer chartCacheStore.Unlock()

	chartCacheStore.data[key] = ChartCacheEntry{
		Data:   data,
		Expire: time.Now().Add(ttl).Unix(),
	}
}

func cleanupExpiredChartCache() {
	ticker := time.NewTicker(cleanupIntervalChart)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().Unix()

		chartCacheStore.Lock()
		for key, entry := range chartCacheStore.data {
			if now > entry.Expire {
				delete(chartCacheStore.data, key)
			}
		}
		chartCacheStore.Unlock()
	}
}
