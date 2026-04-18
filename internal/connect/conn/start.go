package conn

import (
	"context"
	"mosona-manager/internal/connect/active"
	"mosona-manager/internal/connect/ssh"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils/encrypt"
	"time"
)

func StartServer(serverId int64, mode int16) error {
	switch mode {
	case 0:
		var host, user string
		var port int

		var (
			password []byte
			pwdStr   string

			key    []byte
			keyStr string

			keyPassword []byte
			keyPwdStr   string
		)

		if err := db.Db.QueryRow(
			"SELECT address, port, username, ssh.password, k.password, k.content FROM servers s JOIN ssh ON s.id = ssh.server_id LEFT JOIN keys k ON ssh.key_id = k.id WHERE s.id = $1", serverId,
		).Scan(&host, &port, &user, &password, &keyPassword, &key); err != nil {
			return err
		}
		if len(password) != 0 {
			pwd, err := encrypt.Decrypt(password, encrypt.Key)
			if err != nil {
				return err
			}
			pwdStr = string(pwd)
		}
		if len(key) != 0 {
			k, err := encrypt.Decrypt(key, encrypt.Key)
			if err != nil {
				return err
			}
			keyStr = string(k)
		}
		if len(keyPassword) != 0 {
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
			cancel: cancel,
		}
		mu.Unlock()
		go func() {
			_ = ssh.SSH(ctx, host, port, user, pwdStr, keyStr, keyPwdStr, serverId)
		}()
	case 1:
		var (
			agentUid string
			host     string
			port     int
			privKey  string
		)
		if err := db.Db.QueryRow(
			"SELECT agent_uid, host, port, private_key FROM servers s JOIN agents a ON s.id = a.server_id WHERE s.id = $1", serverId,
		).Scan(&agentUid, &host, &port, &privKey); err != nil {
			return err
		}

		mu.Lock()
		if old, exists := connectPool[serverId]; exists {
			old.cancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		connectPool[serverId] = &ServerEntry{
			cancel: cancel,
		}
		mu.Unlock()

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					_ = active.Connect(ctx, host, port, privKey, agentUid, serverId)
				}
			}
		}()
	}

	return nil
}
