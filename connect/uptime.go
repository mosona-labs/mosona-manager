package connect

import (
	"golang.org/x/crypto/ssh"
	"strings"
	"time"
)

func uptime(os string, client *ssh.Client) (time.Time, error) {
	session, err := client.NewSession()
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = session.Close() }()

	var command string
	switch os {
	case "Linux":
		command = "who -b | awk '{print $3, $4}'"
	case "Darwin":
		command = "sysctl -n kern.boottime | awk -F'[=,}]' '{print $2}' | xargs -I{} date -r {} \"+%Y-%m-%d %H:%M:%S\""
	default:
		return time.Time{}, nil
	}

	out, err := session.CombinedOutput(command)
	if err != nil {
		return time.Time{}, err
	}

	thisTime, err := time.Parse("2006-01-02 15:04", strings.TrimSpace(string(out)))
	if err != nil {
		return time.Time{}, err
	}

	return thisTime, nil
}
