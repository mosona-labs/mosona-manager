package connect

import (
	"context"
	"fmt"
	"golang.org/x/crypto/ssh"
	"log"
	"mosona-manager/_type"
	"mosona-manager/config"
	"mosona-manager/db"
	"mosona-manager/influx"
	"mosona-manager/utils"
	"sync"
	"time"
)

var (
	mu          sync.Mutex
	connectPool = make(map[int64]*serverEntry)
)

type serverEntry struct {
	cancel   context.CancelFunc
	host     string
	port     int
	user     string
	password string
	callback func(data _type.ServerStatusType)
	info     func(system string, startTime time.Time)
}

func StartServer(serverId int64) error {
	var host, user string
	var port int
	var password []byte
	var pwdStr string

	if err := db.Db.QueryRow(
		"SELECT address, port, username, password FROM servers WHERE id = $1", serverId,
	).Scan(&host, &port, &user, &password); err != nil {
		return err
	}
	if password != nil {
		pwd, err := utils.Decrypt(password, config.Key)
		if err != nil {
			return err
		}
		pwdStr = string(pwd)
	}

	mu.Lock()
	defer mu.Unlock()

	var callback = func(data _type.ServerStatusType) {
		if err := influx.AddServerStatus(serverId, data); err != nil {
			log.Println("Failed to add server status:", err)
		}
	}
	var info = func(system string, startTime time.Time) {
		if _, err := db.Db.Exec(
			"UPDATE server_info SET os = $1, open_time = $2 WHERE sid = $3",
			system, startTime, serverId,
		); err != nil {
			log.Println("Failed to update server info:", err)
		}
		ipAddress, err := utils.GetDomainAddress(host)
		if err != nil {
			log.Println("Failed to get domain address:", err)
			return
		}
		geo, err := utils.GetIPGeoLocation(ipAddress)
		if err != nil {
			log.Println("Failed to get IP geo location:", err)
			return
		}
		if _, err = db.Db.Exec(
			"UPDATE server_info SET county = $1, area = $2 WHERE sid = $3",
			geo.CountryCode, geo.Country, serverId,
		); err != nil {
			log.Println("Failed to update server info:", err)
		}
	}

	if old, exists := connectPool[serverId]; exists {
		old.cancel()
		ctx, cancel := context.WithCancel(context.Background())
		connectPool[serverId] = &serverEntry{
			cancel:   cancel,
			host:     host,
			port:     port,
			user:     user,
			password: pwdStr,
			callback: callback,
			info:     info,
		}
		go func() {
			_ = SSH(ctx, host, port, user, pwdStr, callback, info)
		}()

		return nil
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		connectPool[serverId] = &serverEntry{
			cancel:   cancel,
			host:     host,
			port:     port,
			user:     user,
			password: pwdStr,
			callback: callback,
			info:     info,
		}

		go func() {
			_ = SSH(ctx, host, port, user, pwdStr, callback, info)
		}()
	}

	return nil
}

func SSH(
	ctx context.Context,
	host string, port int, user, password string,
	callback func(data _type.ServerStatusType),
	info func(system string, startTime time.Time),
) error {
	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 30 * time.Second
		dialTimeout    = 10 * time.Second
	)

	backoff := initialBackoff

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		config := &ssh.ClientConfig{
			User: user,
			Auth: []ssh.AuthMethod{
				ssh.Password(password),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         dialTimeout,
		}

		client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		backoff = initialBackoff

		connClosed := make(chan struct{})
		go func() {
			<-ctx.Done()
			_ = client.Close()
			close(connClosed)
		}()

		workErr := func() error {
			defer func() { _ = client.Close() }()

			osName, err := OS(client)
			if err != nil {
				return err
			}
			switch osName {
			case "Linux":
				system, err := linuxVersion(client)
				if err != nil {
					return err
				}
				startTime, err := uptime("Linux", client)
				if err != nil {
					return err
				}

				info(system, startTime)
				if err = status(client, callback); err != nil {
					return err
				}
			}
			return nil
		}()

		if workErr == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}

		_ = connClosed
	}
}
