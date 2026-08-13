package conn

import (
	"context"
	"sync"
	"time"
)

var (
	mu               sync.Mutex
	retryMu          sync.Mutex
	lifecycleLocks   [64]sync.Mutex
	connectPool      = make(map[int64]*ServerEntry)
	reconcileRetries = make(map[int64]*reconcileRetry)
	inboundStop      func(int64)
	retryTimer       = time.After
)

func lifecycleLock(serverID int64) *sync.Mutex {
	return &lifecycleLocks[uint64(serverID)%uint64(len(lifecycleLocks))]
}

type ServerEntry struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type reconcileRetry struct {
	cancel context.CancelFunc
	done   chan struct{}
	timer  func(time.Duration) <-chan time.Time
}
