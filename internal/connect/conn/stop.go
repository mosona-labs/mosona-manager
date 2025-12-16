package conn

func StopServer(serverId int64) {
	mu.Lock()
	if entry, exists := connectPool[serverId]; exists {
		entry.cancel()
		delete(connectPool, serverId)
	}
	mu.Unlock()
}
