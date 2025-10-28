package connect

import (
	"golang.org/x/crypto/ssh"
	"strings"
)

func OS(client *ssh.Client) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = session.Close() }()
	out, err := session.CombinedOutput("uname -s")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
