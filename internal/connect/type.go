package connect

import (
	"context"
	"mosona-manager/internal/_type"
)

// ServerEntry represents a server connection entry in the connection pool
type ServerEntry struct {
	cancel   context.CancelFunc
	serverId int64
	host     string
	port     int
	user     string
	password string
	key      string
	keyPwd   string
}

// StatusCallback is called when server status data is received
type StatusCallback func(data _type.ServerStatusType)

// ServerInfoCallback is called when server info data is received
type ServerInfoCallback func(data _type.ServerInfoType)
