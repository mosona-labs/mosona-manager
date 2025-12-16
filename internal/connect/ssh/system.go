package ssh

import (
	"strings"

	"golang.org/x/crypto/ssh"
)

func oS(client *ssh.Client) (string, error) {
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
