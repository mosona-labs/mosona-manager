package conn

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"mosona-manager/internal/connect/active"
	"mosona-manager/internal/connect/ssh"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils/encrypt"
	"time"
)

var (
	connectActiveAgent            = active.Connect
	activeConnectionRetryTimer    = time.After
	logLegacyAgentUpgradeRequired = func(serverID int64) {
		log.Printf("Active Agent for server %d is online via the legacy handshake; waiting for the Agent to update before authenticated pairing", serverID)
	}
)

func ReconcileServer(serverId int64) error {
	lock := lifecycleLock(serverId)
	lock.Lock()
	defer lock.Unlock()
	cancelReconcileRetry(serverId)
	err := reconcileServer(serverId)
	if err != nil {
		startReconcileRetry(serverId)
	}
	return err
}

func reconcileServer(serverId int64) error {
	var mode int16
	var allowMonitor bool
	if err := db.Db.QueryRow(
		"SELECT type, allow_monitor FROM servers WHERE id = $1",
		serverId,
	).Scan(&mode, &allowMonitor); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			stopServer(serverId)
			return nil
		}
		stopServer(serverId)
		return err
	}

	if !allowMonitor {
		if mode != 1 {
			stopServer(serverId)
			return nil
		}
		var publicKey string
		if err := db.Db.QueryRow("SELECT public_key FROM agents WHERE server_id = $1", serverId).Scan(&publicKey); err != nil {
			stopServer(serverId)
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		if publicKey != "" {
			stopServer(serverId)
			return nil
		}
	}

	switch mode {
	case 0, 1:
		stopInboundServer(serverId)
		stopOutboundServer(serverId)
		err := startServer(serverId, mode, allowMonitor)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	case 2:
		stopOutboundServer(serverId)
		return nil
	default:
		stopServer(serverId)
		return fmt.Errorf("unsupported server connection type %d", mode)
	}
}

func cancelReconcileRetry(serverID int64) {
	retryMu.Lock()
	defer retryMu.Unlock()
	if retry, exists := reconcileRetries[serverID]; exists {
		retry.cancel()
		delete(reconcileRetries, serverID)
	}
}

func startReconcileRetry(serverID int64) {
	ctx, cancel := context.WithCancel(context.Background())
	retry := &reconcileRetry{
		cancel: cancel,
		done:   make(chan struct{}),
		timer:  retryTimer,
	}
	retryMu.Lock()
	reconcileRetries[serverID] = retry
	retryMu.Unlock()
	go func() {
		defer close(retry.done)
		failures := 0
		for {
			delay := retryDelay(failures, rand.Float64())
			select {
			case <-ctx.Done():
				return
			case <-retry.timer(delay):
			}

			lock := lifecycleLock(serverID)
			lock.Lock()
			retryMu.Lock()
			current := reconcileRetries[serverID]
			retryMu.Unlock()
			if ctx.Err() != nil || current != retry {
				lock.Unlock()
				return
			}
			err := reconcileServer(serverID)
			if err == nil {
				retryMu.Lock()
				if reconcileRetries[serverID] == retry {
					delete(reconcileRetries, serverID)
				}
				retryMu.Unlock()
				lock.Unlock()
				return
			}
			failures++
			lock.Unlock()
			log.Printf("Failed to reconcile monitoring for server %d; retrying after %s: %v", serverID, retryDelay(failures, 0), err)
		}
	}()
}

func startServer(serverId int64, mode int16, allowMonitor bool) error {
	switch mode {
	case 0:
		var host, user string
		var trustedHostKey sql.NullString
		var trustLegacyHostKey bool
		var teamID int64
		var port int

		var (
			password []byte
			pwdStr   string

			key    []byte
			keyStr string

			keyPassword []byte
			keyPwdStr   string
			keyID       int64
		)

		if err := db.Db.QueryRow(
			"SELECT address, port, username, ssh.password, COALESCE(ssh.key_id, 0), k.password, k.content, ssh.host_key, ssh.trust_legacy_host_key, s.team_id FROM servers s JOIN ssh ON s.id = ssh.server_id LEFT JOIN keys k ON ssh.key_id = k.id WHERE s.id = $1", serverId,
		).Scan(&host, &port, &user, &password, &keyID, &keyPassword, &key, &trustedHostKey, &trustLegacyHostKey, &teamID); err != nil {
			return err
		}
		if len(password) != 0 {
			pwd, err := encrypt.Decrypt(password, encrypt.Key, encrypt.SSHPasswordContext(serverId))
			if err != nil {
				return err
			}
			pwdStr = string(pwd)
		}
		if len(key) != 0 {
			k, err := encrypt.Decrypt(key, encrypt.Key, encrypt.KeyContentContext(keyID))
			if err != nil {
				return err
			}
			keyStr = string(k)
		}
		if len(keyPassword) != 0 {
			kp, err := encrypt.Decrypt(keyPassword, encrypt.Key, encrypt.KeyPasswordContext(keyID))
			if err != nil {
				return err
			}
			keyPwdStr = string(kp)
		}

		stopOutboundServer(serverId)
		mu.Lock()
		ctx, cancel := context.WithCancel(context.Background())
		connectPool[serverId] = &ServerEntry{
			cancel: cancel,
			done:   make(chan struct{}),
		}
		entry := connectPool[serverId]
		mu.Unlock()
		go func() {
			defer finishOutboundServer(serverId, entry)
			_ = ssh.SSH(ctx, host, port, user, pwdStr, keyStr, keyPwdStr, trustedHostKey.String, trustLegacyHostKey, serverId, teamID)
		}()
	case 1:
		var (
			agentUid        string
			host            string
			port            int
			privKey         string
			publicKey       string
			protocolVersion int16
		)
		if err := db.Db.QueryRow(
			"SELECT agent_uid, host, port, private_key, public_key, protocol_version FROM servers s JOIN agents a ON s.id = a.server_id WHERE s.id = $1", serverId,
		).Scan(&agentUid, &host, &port, &privKey, &publicKey, &protocolVersion); err != nil {
			return err
		}

		stopOutboundServer(serverId)
		mu.Lock()
		ctx, cancel := context.WithCancel(context.Background())
		connectPool[serverId] = &ServerEntry{
			cancel: cancel,
			done:   make(chan struct{}),
		}
		entry := connectPool[serverId]
		mu.Unlock()

		go func() {
			defer finishOutboundServer(serverId, entry)
			failures := 0
			legacyUpgradeNoticed := false
			for {
				delay := retryDelay(failures, rand.Float64())
				select {
				case <-ctx.Done():
					return
				case <-activeConnectionRetryTimer(delay):
				}
				startedAt := time.Now()
				err := connectActiveAgent(ctx, host, port, privKey, agentUid, publicKey, protocolVersion, serverId, allowMonitor)
				if ctx.Err() != nil {
					return
				}
				if errors.Is(err, active.ErrAgentIdentityMismatch) {
					return
				}
				if errors.Is(err, active.ErrLegacyAgentRequiresUpgrade) && !legacyUpgradeNoticed {
					logLegacyAgentUpgradeRequired(serverId)
					legacyUpgradeNoticed = true
				}
				if currentErr := db.Db.QueryRow("SELECT public_key, protocol_version FROM agents WHERE server_id = $1", serverId).Scan(&publicKey, &protocolVersion); currentErr != nil {
					log.Printf("Failed to reload Active Agent identity for server %d: %v", serverId, currentErr)
				}
				if err == nil && !allowMonitor {
					return
				}
				if err != nil {
					if time.Since(startedAt) >= time.Minute {
						failures = 0
					} else {
						failures++
					}
				}
			}
		}()
	}

	return nil
}

func retryDelay(failures int, jitter float64) time.Duration {
	delay := time.Second
	for i := 0; i < failures && delay < time.Minute; i++ {
		delay *= 2
		if delay > time.Minute {
			delay = time.Minute
		}
	}
	if jitter < 0 {
		jitter = 0
	} else if jitter > 1 {
		jitter = 1
	}
	return delay + time.Duration(float64(delay)*0.25*jitter)
}
