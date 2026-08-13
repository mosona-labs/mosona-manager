package ssh

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

var errSSHCertificateRequiresPrivateKey = errors.New("ssh certificate requires a matching private key")

var (
	ErrHostKeyMissing                 = errors.New("ssh host key is not trusted")
	ErrHostKeyMismatch                = errors.New("ssh host key does not match the trusted key")
	ErrHostKeyInvalid                 = errors.New("trusted ssh host key is invalid")
	ErrHostKeyChangedDuringValidation = errors.New("ssh host key changed during connection validation")
)

type HostKeyTrustError struct {
	Kind                error
	ExpectedFingerprint string
	ActualFingerprint   string
}

func (e *HostKeyTrustError) Error() string {
	switch {
	case errors.Is(e.Kind, ErrHostKeyMismatch):
		return fmt.Sprintf("%v (expected %s, received %s)", e.Kind, e.ExpectedFingerprint, e.ActualFingerprint)
	case e.ActualFingerprint != "":
		return fmt.Sprintf("%v (received %s)", e.Kind, e.ActualFingerprint)
	default:
		return e.Kind.Error()
	}
}

func (e *HostKeyTrustError) Unwrap() error { return e.Kind }

type ObservedHostKey struct {
	AuthorizedKey string
	Fingerprint   string
}

const DefaultDialTimeout = 10 * time.Second
const keepAliveInterval = 20 * time.Second

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

func Dial(host string, port int, user, password, key, keyPwd, trustedHostKey string, trustLegacyHostKey bool, timeout time.Duration) (*gossh.Client, error) {
	hostKeyCallback, err := hostKeyCallback(trustedHostKey, trustLegacyHostKey)
	if err != nil {
		return nil, err
	}

	authMethods, err := BuildAuthMethods(password, key, keyPwd)
	if err != nil {
		return nil, err
	}

	client, err := gossh.Dial("tcp", sshAddress(host, port), &gossh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	})
	if err != nil {
		return nil, err
	}

	return client, nil
}

func ValidateConnection(host string, port int, user, password, key, keyPwd, trustedHostKey, confirmedHostKey string) (ObservedHostKey, error) {
	return validateConnection(
		host, port, user, password, key, keyPwd, trustedHostKey, confirmedHostKey,
		ProbeHostKey,
		func(acceptedHostKey string) error {
			client, err := Dial(host, port, user, password, key, keyPwd, acceptedHostKey, false, DefaultDialTimeout)
			if err != nil {
				return err
			}
			_ = client.Close()
			return nil
		},
	)
}

func validateConnection(
	host string,
	port int,
	user, password, key, keyPwd, trustedHostKey, confirmedHostKey string,
	probe func(string, int, time.Duration) (ObservedHostKey, error),
	authenticate func(string) error,
) (ObservedHostKey, error) {
	observed, err := probe(host, port, DefaultDialTimeout)
	if err != nil {
		return ObservedHostKey{}, fmt.Errorf("failed to inspect host key for %s: %w", sshAddress(host, port), err)
	}

	acceptedHostKey, err := verifyObservedHostKey(trustedHostKey, confirmedHostKey, observed)
	if err != nil {
		return observed, err
	}

	if err = authenticate(acceptedHostKey); err != nil {
		if IsHostKeyTrustError(err) {
			return observed, fmt.Errorf("%w: %v", ErrHostKeyChangedDuringValidation, err)
		}
		return observed, fmt.Errorf("failed to connect to %s as %s: %w", sshAddress(host, port), user, err)
	}

	return observed, nil
}

func ProbeHostKey(host string, port int, timeout time.Duration) (ObservedHostKey, error) {
	address := sshAddress(host, port)
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return ObservedHostKey{}, err
	}
	return probeHostKeyConnection(conn, address, timeout)
}

func probeHostKeyConnection(conn net.Conn, address string, timeout time.Duration) (ObservedHostKey, error) {
	defer func() { _ = conn.Close() }()
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}

	var observed ObservedHostKey
	stopAfterHostKey := errors.New("ssh host key captured")
	config := &gossh.ClientConfig{
		User: "host-key-probe",
		HostKeyCallback: func(_ string, _ net.Addr, key gossh.PublicKey) error {
			observed = observedHostKey(key)
			return stopAfterHostKey
		},
		Timeout: timeout,
	}
	_, _, _, err := gossh.NewClientConn(conn, address, config)
	if observed.AuthorizedKey != "" && errors.Is(err, stopAfterHostKey) {
		return observed, nil
	}
	if err == nil {
		return ObservedHostKey{}, errors.New("ssh handshake completed without a host key")
	}
	return ObservedHostKey{}, err
}

func hostKeyCallback(trustedHostKey string, trustLegacyHostKey bool) (gossh.HostKeyCallback, error) {
	if strings.TrimSpace(trustedHostKey) == "" && trustLegacyHostKey {
		return gossh.InsecureIgnoreHostKey(), nil
	}
	return strictHostKeyCallback(trustedHostKey)
}

func strictHostKeyCallback(trustedHostKey string) (gossh.HostKeyCallback, error) {
	trusted, err := parseTrustedHostKey(trustedHostKey)
	if err != nil {
		return nil, err
	}

	return func(_ string, _ net.Addr, actual gossh.PublicKey) error {
		if bytes.Equal(trusted.Marshal(), actual.Marshal()) {
			return nil
		}
		return &HostKeyTrustError{
			Kind:                ErrHostKeyMismatch,
			ExpectedFingerprint: gossh.FingerprintSHA256(trusted),
			ActualFingerprint:   gossh.FingerprintSHA256(actual),
		}
	}, nil
}

func parseTrustedHostKey(value string) (gossh.PublicKey, error) {
	if strings.TrimSpace(value) == "" {
		return nil, &HostKeyTrustError{Kind: ErrHostKeyMissing}
	}
	key, _, _, rest, err := gossh.ParseAuthorizedKey([]byte(value))
	if err != nil || len(bytes.TrimSpace(rest)) != 0 {
		if err == nil {
			err = errors.New("unexpected data after public key")
		}
		return nil, fmt.Errorf("%w: %v", ErrHostKeyInvalid, err)
	}
	return key, nil
}

func verifyObservedHostKey(trustedHostKey, confirmedHostKey string, observed ObservedHostKey) (string, error) {
	if trustedHostKey != "" {
		trusted, err := parseTrustedHostKey(trustedHostKey)
		if err != nil && !errors.Is(err, ErrHostKeyInvalid) {
			return "", err
		}
		if err == nil {
			actual, err := parseTrustedHostKey(observed.AuthorizedKey)
			if err != nil {
				return "", err
			}
			if bytes.Equal(trusted.Marshal(), actual.Marshal()) {
				return observed.AuthorizedKey, nil
			}
		}
	}

	if confirmedHostKey == "" {
		kind := ErrHostKeyMissing
		expectedFingerprint := ""
		if trustedHostKey != "" {
			kind = ErrHostKeyMismatch
			trusted, err := parseTrustedHostKey(trustedHostKey)
			if err != nil && !errors.Is(err, ErrHostKeyInvalid) {
				return "", err
			}
			if err == nil {
				expectedFingerprint = gossh.FingerprintSHA256(trusted)
			}
		}
		return "", &HostKeyTrustError{
			Kind:                kind,
			ExpectedFingerprint: expectedFingerprint,
			ActualFingerprint:   observed.Fingerprint,
		}
	}

	confirmed, err := parseTrustedHostKey(confirmedHostKey)
	if err != nil {
		return "", err
	}
	actual, err := parseTrustedHostKey(observed.AuthorizedKey)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(confirmed.Marshal(), actual.Marshal()) {
		return "", &HostKeyTrustError{
			Kind:                ErrHostKeyMismatch,
			ExpectedFingerprint: gossh.FingerprintSHA256(confirmed),
			ActualFingerprint:   observed.Fingerprint,
		}
	}
	return observed.AuthorizedKey, nil
}

func observedHostKey(key gossh.PublicKey) ObservedHostKey {
	return ObservedHostKey{
		AuthorizedKey: strings.TrimSpace(string(gossh.MarshalAuthorizedKey(key))),
		Fingerprint:   gossh.FingerprintSHA256(key),
	}
}

func IsHostKeyTrustError(err error) bool {
	return errors.Is(err, ErrHostKeyMissing) || errors.Is(err, ErrHostKeyMismatch)
}

func IsPermanentHostKeyError(err error) bool {
	return IsHostKeyTrustError(err) ||
		errors.Is(err, ErrHostKeyInvalid) ||
		errors.Is(err, ErrHostKeyChangedDuringValidation)
}

func HostKeysEqual(first, second string) bool {
	firstKey, err := parseTrustedHostKey(first)
	if err != nil {
		return false
	}
	secondKey, err := parseTrustedHostKey(second)
	return err == nil && bytes.Equal(firstKey.Marshal(), secondKey.Marshal())
}

func NormalizeHostKey(value string) (string, error) {
	key, err := parseTrustedHostKey(value)
	if err != nil {
		return "", err
	}
	return observedHostKey(key).AuthorizedKey, nil
}

func sshAddress(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func KeepAlive(ctx context.Context, client *gossh.Client) {
	ticker := time.NewTicker(keepAliveInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _, err := client.SendRequest("keepalive@openssh.com", false, nil)
				if err != nil {
					log.Println("ssh keepalive:", err)
					_ = client.Close()
					return
				}
			}
		}
	}()
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
