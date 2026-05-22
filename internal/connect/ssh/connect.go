package ssh

import (
	"context"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect/callback"
	"strconv"
	"time"
)

func SSH(
	ctx context.Context,
	host string, port int, user, password, key, keyPwd string,
	serverId int64,
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

		client, err := Dial(host, port, user, password, key, keyPwd, DefaultDialTimeout)

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
					callback.Information(
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
					)
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
