package ssh

import (
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

var errSSHCertificateRequiresPrivateKey = errors.New("ssh certificate requires a matching private key")

const defaultDialTimeout = 10 * time.Second

func BuildAuthMethods(password, key, keyPwd string) ([]gossh.AuthMethod, error) {
	if key == "" {
		return []gossh.AuthMethod{gossh.Password(password)}, nil
	}

	signer, err := parseSigner(key, keyPwd)
	if err != nil {
		return nil, err
	}

	authMethods := []gossh.AuthMethod{gossh.PublicKeys(signer)}
	if password != "" {
		authMethods = append(authMethods, gossh.Password(password))
	}

	return authMethods, nil
}

func Dial(host string, port int, user, password, key, keyPwd string, timeout time.Duration) (*gossh.Client, error) {
	authMethods, err := BuildAuthMethods(password, key, keyPwd)
	if err != nil {
		return nil, err
	}

	client, err := gossh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), &gossh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	})
	if err != nil {
		return nil, err
	}

	return client, nil
}

func ValidateConnection(host string, port int, user, password, key, keyPwd string) error {
	client, err := Dial(host, port, user, password, key, keyPwd, defaultDialTimeout)
	if err != nil {
		return fmt.Errorf("failed to connect to %s:%d as %s: %w", host, port, user, err)
	}
	defer func() {
		_ = client.Close()
	}()

	return nil
}

func parseSigner(key, keyPwd string) (gossh.Signer, error) {
	cert, err := findOpenSSHCertificate(key)
	if err != nil {
		return nil, err
	}

	privateKeyPEM := findPrivateKeyPEM([]byte(key))
	if len(privateKeyPEM) == 0 {
		if cert != nil {
			return nil, errSSHCertificateRequiresPrivateKey
		}
		return nil, errors.New("ssh: no private key found")
	}

	var signer gossh.Signer
	if keyPwd != "" {
		signer, err = gossh.ParsePrivateKeyWithPassphrase(privateKeyPEM, []byte(keyPwd))
	} else {
		signer, err = gossh.ParsePrivateKey(privateKeyPEM)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	if cert == nil {
		return signer, nil
	}

	certSigner, err := gossh.NewCertSigner(cert, signer)
	if err != nil {
		return nil, fmt.Errorf("failed to pair private key with ssh certificate: %w", err)
	}

	return certSigner, nil
}

func findPrivateKeyPEM(content []byte) []byte {
	rest := content
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil {
			return nil
		}

		switch block.Type {
		case "RSA PRIVATE KEY", "PRIVATE KEY", "EC PRIVATE KEY", "DSA PRIVATE KEY", "OPENSSH PRIVATE KEY":
			return pem.EncodeToMemory(block)
		}

		rest = next
	}

	return nil
}

func findOpenSSHCertificate(content string) (*gossh.Certificate, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		pubKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			if strings.Contains(line, "-cert-v01@openssh.com") {
				return nil, fmt.Errorf("failed to parse ssh certificate: %w", err)
			}
			continue
		}

		cert, ok := pubKey.(*gossh.Certificate)
		if ok {
			return cert, nil
		}
	}

	return nil, nil
}
