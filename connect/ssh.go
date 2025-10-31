package connect

import (
	"context"
	"fmt"
	"mosona-manager/_type"
	"time"

	"golang.org/x/crypto/ssh"
)

func SSH(
	ctx context.Context,
	host string, port int, user, password string,
	callback func(data _type.ServerStatusType),
	info infoCallbackType,
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

		client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), &ssh.ClientConfig{
			User: user,
			Auth: []ssh.AuthMethod{
				ssh.Password(password),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         dialTimeout,
		})
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

			osName, err := oS(client)
			if err != nil {
				return err
			}
			switch osName {
			case "Linux":
				if err = information(client, func(data _type.ServerInfoType) {
					bootTime, _ := time.Parse(
						"2006-01-02 15:04",
						data.Uptime,
					)
					info(
						data.LinuxVersion,
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
				if err = status(client, callback); err != nil {
					return err
				}
			}
			return nil
		}()

		if workErr == nil {
			return nil
		} else {
			fmt.Println(workErr)
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
