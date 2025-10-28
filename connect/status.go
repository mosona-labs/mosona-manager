package connect

import (
	"bufio"
	"encoding/json"
	"golang.org/x/crypto/ssh"
	"io"
	"mosona-manager/_type"
	"os"
)

func status(client *ssh.Client, callback func(data _type.ServerStatusType)) error {
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
			callback(data)
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

	scriptFile, err := os.ReadFile(`./connect/linux.sh`)
	if err != nil {
		_ = stdin.Close()
		return err
	}
	_, _ = io.WriteString(stdin, string(scriptFile))
	_ = stdin.Close()

	if err = session.Wait(); err != nil {
		<-done
		return err
	}
	<-done
	return nil
}
