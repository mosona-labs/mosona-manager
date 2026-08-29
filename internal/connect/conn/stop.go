package conn

func RegisterInboundStopper(stop func(int64)) {
	mu.Lock()
	inboundStop = stop
	mu.Unlock()
}

func StopServer(serverId int64) {
	lock := lifecycleLock(serverId)
	lock.Lock()
	defer lock.Unlock()
	cancelReconcileRetry(serverId)
	stopServer(serverId)
}

func stopServer(serverId int64) {
	stopOutboundServer(serverId)
	stopInboundServer(serverId)
}

func stopOutboundServer(serverId int64) {
	mu.Lock()
	entry, exists := connectPool[serverId]
	if exists {
		delete(connectPool, serverId)
	}
	mu.Unlock()
	if exists {
		entry.cancel()
		if entry.done != nil {
			<-entry.done
		}
	}
}

func finishOutboundServer(serverID int64, entry *ServerEntry) {
	mu.Lock()
	if connectPool[serverID] == entry {
		delete(connectPool, serverID)
	}
	close(entry.done)
	mu.Unlock()
}

func stopInboundServer(serverId int64) {
	mu.Lock()
	stopInbound := inboundStop
	mu.Unlock()
	if stopInbound != nil {
		stopInbound(serverId)
	}
}
