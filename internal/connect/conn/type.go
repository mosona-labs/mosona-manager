package conn

import (
	"context"
	"sync"
)

var (
	mu          sync.Mutex
	connectPool = make(map[int64]*ServerEntry)
)

type ServerEntry struct {
	cancel context.CancelFunc
}
