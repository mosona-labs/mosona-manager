package ssh

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect/ssh/script"
	"mosona-manager/internal/influx"
	"sync"

	"golang.org/x/crypto/ssh"
)

var errStatusMonitorEnded = errors.New("ssh status monitor ended unexpectedly")

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

	stdoutScanner := newLineScanner(stdout)
	stderrScanner := newLineScanner(stderr)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for stdoutScanner.Scan() {
			var data _type.ServerStatusType
			if err := json.Unmarshal([]byte(stdoutScanner.Text()), &data); err != nil {
				continue
			}
			if err := influx.AddServerStatus(serverId, data); err != nil {
				log.Println("Failed to add server status:", err)
			}
		}
		if err := stdoutScanner.Err(); err != nil {
			log.Println("ssh status stdout:", err)
		}
	}()
	go func() {
		defer wg.Done()
		for stderrScanner.Scan() {
			log.Println("ssh status stderr:", stderrScanner.Text())
		}
		if err := stderrScanner.Err(); err != nil {
			log.Println("ssh status stderr:", err)
		}
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
		wg.Wait()
		return fmt.Errorf("script error: %w", err)
	}
	wg.Wait()
	return errStatusMonitorEnded
}
