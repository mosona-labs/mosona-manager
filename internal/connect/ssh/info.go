package ssh

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect/ssh/script"
	"sync"

	"golang.org/x/crypto/ssh"
)

func information(client *ssh.Client, callback func(data _type.ServerInfoType)) error {
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
			var data _type.ServerInfoType
			if err := json.Unmarshal([]byte(stdoutScanner.Text()), &data); err != nil {
				continue
			}
			callback(data)
		}
		if err := stdoutScanner.Err(); err != nil {
			log.Println("ssh info stdout:", err)
		}
	}()
	go func() {
		defer wg.Done()
		for stderrScanner.Scan() {
			log.Println("ssh info stderr:", stderrScanner.Text())
		}
		if err := stderrScanner.Err(); err != nil {
			log.Println("ssh info stderr:", err)
		}
	}()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	if err = session.Start("bash -s"); err != nil {
		return err
	}

	scriptFile, err := script.GetScript("linux_info.sh")
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
	return nil
}
