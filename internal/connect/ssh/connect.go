package ssh

import (
	"context"
	"fmt"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect/callback"
	"mosona-manager/internal/influx"
	"strconv"
	"time"
)

func SSH(
	ctx context.Context,
	host string, port int, user, password, key, keyPwd, trustedHostKey string,
	serverId, teamID int64,
) error {
	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 30 * time.Second
	)

	backoff := initialBackoff

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		client, err := Dial(host, port, user, password, key, keyPwd, trustedHostKey, DefaultDialTimeout)

		if err != nil {
			if IsPermanentHostKeyError(err) {
				log.Printf("Blocked SSH connection for server %d: %v", serverId, err)
				influx.LogAdd(
					teamID, 0, "security",
					fmt.Sprintf("Blocked SSH connection due to untrusted host key (server ID %d): %v", serverId, err),
					"", "monitoring service", "high",
				)
				return err
			}
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
		connCtx, connCancel := context.WithCancel(ctx)
		KeepAlive(connCtx, client)

		go func() {
			<-connCtx.Done()
			_ = client.Close()
		}()

		workErr := func() error {
			defer connCancel()
			defer func() { _ = client.Close() }()

			osName, err := oS(client)
			if err != nil {
				return err
			}
			switch osName {
			case "Linux":
				if err = information(client, func(data _type.ServerInfoType) {
					u, _ := strconv.ParseInt(data.Uptime, 10, 64)
					bootTime := time.Unix(time.Now().Unix()-u, 0)
					if err := callback.Information(
						serverId,
						host,
						data.SystemVersion,
						bootTime,
						data.Hostname,
						data.CpuName,
						data.CpuC,
						data.CpuT,
						data.KernelVersion,
						data.IPAddress,
						data.Architecture,
					); err != nil {
						log.Println("Failed to update SSH server information:", err)
					}
				}); err != nil {
					return err
				}
				if err = status(client, serverId); err != nil {
					return err
				}
			}
			return nil
		}()

		if workErr == nil {
			return nil
		} else {
			log.Println(workErr)
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

	}
}
