package connect

import (
	"golang.org/x/crypto/ssh"
	"strings"
)

func linuxVersion(client *ssh.Client) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = session.Close() }()

	out, err := session.CombinedOutput("[ -f /etc/os-release ] && . /etc/os-release && distro_name=\"$NAME\" || { [ -f /etc/redhat-release ] && distro_name=\"$(cat /etc/redhat-release)\" || distro_name=\"Linux\"; }; echo \"$distro_name\"")
	if err != nil {
		return "", err
	}
	system := strings.ToLower(strings.Split(strings.TrimSpace(string(out)), " ")[0])

	return system, nil
}
