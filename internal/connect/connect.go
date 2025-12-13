package connect

import (
	"context"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils/encrypt"
	"sync"
)

var (
	mu          sync.Mutex
	connectPool = make(map[int64]*ServerEntry)
)

func StartServer(serverId int64) error {
	var host, user string
	var port int

	var (
		password []byte
		pwdStr   string
	)
	var (
		key    []byte
		keyStr string
	)
	var (
		keyPassword []byte
		keyPwdStr   string
	)

	if err := db.Db.QueryRow(
		"SELECT address, port, username, ssh.password, k.password, k.content FROM servers s JOIN ssh ON s.id = ssh.server_id LEFT JOIN keys k ON ssh.key_id = k.id WHERE s.id = $1", serverId,
	).Scan(&host, &port, &user, &password, &keyPassword, &key); err != nil {
		return err
	}
	if password != nil {
		pwd, err := encrypt.Decrypt(password, encrypt.Key)
		if err != nil {
			return err
		}
		pwdStr = string(pwd)
	}
	if key != nil {
		k, err := encrypt.Decrypt(key, encrypt.Key)
		if err != nil {
			return err
		}
		keyStr = string(k)
	}
	if keyPassword != nil {
		kp, err := encrypt.Decrypt(keyPassword, encrypt.Key)
		if err != nil {
			return err
		}
		keyPwdStr = string(kp)
	}

	mu.Lock()
	if old, exists := connectPool[serverId]; exists {
		old.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	connectPool[serverId] = &ServerEntry{
		cancel:   cancel,
		host:     host,
		port:     port,
		user:     user,
		password: pwdStr,
		key:      keyStr,
		keyPwd:   keyPwdStr,
	}
	mu.Unlock()
	go func() {
		_ = SSH(ctx, host, port, user, pwdStr, keyStr, keyPwdStr, serverId)
	}()

	return nil
}

func StopServer(serverId int64) {
	mu.Lock()
	if entry, exists := connectPool[serverId]; exists {
		entry.cancel()
		delete(connectPool, serverId)
	}
	mu.Unlock()
}
