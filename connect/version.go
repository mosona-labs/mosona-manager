package connect

import (
	"bytes"
	"golang.org/x/crypto/ssh"
	"io"
	"strings"
)

func linuxVersion(client *ssh.Client) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = session.Close() }()

	script := `if [ -f /etc/os-release ]; then
. /etc/os-release
distro_name=$NAME
elif [ -f /etc/redhat-release ]; then
distro_name=$(cat /etc/redhat-release)
else
distro_name="Linux"
fi
echo $distro_name`

	var outBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &outBuf
	stdin, err := session.StdinPipe()
	if err != nil {
		return "", err
	}
	if err = session.Start("sh -s"); err != nil {
		return "", err
	}
	_, _ = io.WriteString(stdin, script)
	_ = stdin.Close()
	if err = session.Wait(); err != nil {
		return "", err
	}
	system := strings.ToLower(strings.Split(strings.TrimSpace(outBuf.String()), " ")[0])

	return system, nil
}
