package connect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect/script"
	"mosona-manager/internal/influx"

	"golang.org/x/crypto/ssh"
)

func status(client *ssh.Client, serverId int64) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	reader := io.MultiReader(stdout, stderr)
	scanner := bufio.NewScanner(reader)
	done := make(chan struct{})
	go func() {
		for scanner.Scan() {
			var data _type.ServerStatusType
			if err = json.Unmarshal([]byte(scanner.Text()), &data); err != nil {
				continue
			}
			if err = influx.AddServerStatus(serverId, data); err != nil {
				log.Println("Failed to add server status:", err)
			}
		}
		close(done)
	}()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	if err = session.Start("bash -s"); err != nil {
		return err
	}

	scriptFile, err := script.GetScript("linux_status.sh")
	if err != nil {
		_ = stdin.Close()
		return err
	}
	_, _ = io.WriteString(stdin, scriptFile)
	_ = stdin.Close()

	if err = session.Wait(); err != nil {
		<-done
		return fmt.Errorf("script error: %w", err)
	}
	<-done
	return nil
}
