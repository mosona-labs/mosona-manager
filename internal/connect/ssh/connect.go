package ssh

import (
	"context"
	"fmt"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect/callback"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

func SSH(
	ctx context.Context,
	host string, port int, user, password, key, keyPwd string,
	serverId int64,
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

		var authMethods []ssh.AuthMethod
		if key != "" {
			var signer ssh.Signer
			var err error

			if keyPwd != "" {
				signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(key), []byte(keyPwd))
			} else {
				signer, err = ssh.ParsePrivateKey([]byte(key))
			}
			if err != nil {
				return fmt.Errorf("failed to parse private key: %w", err)
			}
			authMethods = []ssh.AuthMethod{ssh.PublicKeys(signer)}
			if password != "" {
				authMethods = append(authMethods, ssh.Password(password))
			}
		} else {
			authMethods = []ssh.AuthMethod{ssh.Password(password)}
		}

		client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), &ssh.ClientConfig{
			User:            user,
			Auth:            authMethods,
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

		_ = connClosed
	}
}
