package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func TestParseSignerSupportsPrivateKeyOnly(t *testing.T) {
	privateKeyPEM, _ := newTestPrivateKeyAndCertificate(t)

	signer, err := parseSigner(privateKeyPEM, "")
	if err != nil {
		t.Fatalf("parseSigner() error = %v", err)
	}

	if strings.Contains(signer.PublicKey().Type(), "-cert-v01@openssh.com") {
		t.Fatalf("expected plain private key signer, got %q", signer.PublicKey().Type())
	}
}

func TestParseSignerSupportsPrivateKeyAndCertificateInAnyOrder(t *testing.T) {
	privateKeyPEM, certificate := newTestPrivateKeyAndCertificate(t)

	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "certificate_after_private_key",
			content: privateKeyPEM + "\n" + certificate,
		},
		{
			name:    "certificate_before_private_key",
			content: certificate + "\n" + privateKeyPEM,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signer, err := parseSigner(tt.content, "")
			if err != nil {
				t.Fatalf("parseSigner() error = %v", err)
			}

			if !strings.Contains(signer.PublicKey().Type(), "-cert-v01@openssh.com") {
				t.Fatalf("expected certificate signer, got %q", signer.PublicKey().Type())
			}
		})
	}
}

func TestParseSignerRejectsCertificateWithoutPrivateKey(t *testing.T) {
	_, certificate := newTestPrivateKeyAndCertificate(t)

	_, err := parseSigner(certificate, "")
	if !errors.Is(err, errSSHCertificateRequiresPrivateKey) {
		t.Fatalf("expected %v, got %v", errSSHCertificateRequiresPrivateKey, err)
	}
}

func TestParseSignerRejectsMismatchedCertificate(t *testing.T) {
	privateKeyPEM, _ := newTestPrivateKeyAndCertificate(t)
	_, certificate := newTestPrivateKeyAndCertificate(t)

	_, err := parseSigner(privateKeyPEM+"\n"+certificate, "")
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to pair private key with ssh certificate") {
		t.Fatalf("expected cert mismatch error, got %v", err)
	}
}

func TestSSHAddress(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{
			name: "ipv4",
			host: "192.0.2.10",
			port: 22,
			want: "192.0.2.10:22",
		},
		{
			name: "hostname",
			host: "example.com",
			port: 2222,
			want: "example.com:2222",
		},
		{
			name: "ipv6",
			host: "2a14:67c0:308:3::a",
			port: 22,
			want: "[2a14:67c0:308:3::a]:22",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sshAddress(tt.host, tt.port); got != tt.want {
				t.Fatalf("sshAddress(%q, %d) = %q, want %q", tt.host, tt.port, got, tt.want)
			}
		})
	}
}

func TestStrictHostKeyCallback(t *testing.T) {
	trusted := newTestHostKey(t)
	other := newTestHostKey(t)
	trustedText := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(trusted)))

	callback, err := strictHostKeyCallback(trustedText)
	if err != nil {
		t.Fatal(err)
	}
	if err = callback("test", nil, trusted); err != nil {
		t.Fatalf("matching host key rejected: %v", err)
	}
	if err = callback("test", nil, other); !errors.Is(err, ErrHostKeyMismatch) {
		t.Fatalf("mismatched host key error = %v", err)
	}
	if _, err = strictHostKeyCallback(""); !errors.Is(err, ErrHostKeyMissing) {
		t.Fatalf("empty host key error = %v", err)
	}
}

func TestProbeHostKeyStopsBeforeAuthentication(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	serverFile := os.NewFile(uintptr(fds[0]), "ssh-probe-server")
	clientFile := os.NewFile(uintptr(fds[1]), "ssh-probe-client")
	serverConn, err := net.FileConn(serverFile)
	if err != nil {
		t.Fatal(err)
	}
	clientConn, err := net.FileConn(clientFile)
	if err != nil {
		t.Fatal(err)
	}
	_ = serverFile.Close()
	_ = clientFile.Close()

	serverSigner := newTestSigner(t)
	var authCallbacks atomic.Int32
	serverConfig := &gossh.ServerConfig{
		PasswordCallback: func(gossh.ConnMetadata, []byte) (*gossh.Permissions, error) {
			authCallbacks.Add(1)
			return nil, errors.New("authentication must not run during host key probe")
		},
		PublicKeyCallback: func(gossh.ConnMetadata, gossh.PublicKey) (*gossh.Permissions, error) {
			authCallbacks.Add(1)
			return nil, errors.New("authentication must not run during host key probe")
		},
	}
	serverConfig.AddHostKey(serverSigner)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		defer func() { _ = serverConn.Close() }()
		_, _, _, _ = gossh.NewServerConn(serverConn, serverConfig)
	}()

	observed, err := probeHostKeyConnection(clientConn, "socketpair", time.Second)
	if err != nil {
		t.Fatalf("probeHostKeyConnection() error = %v", err)
	}
	if observed.Fingerprint != gossh.FingerprintSHA256(serverSigner.PublicKey()) {
		t.Fatalf("fingerprint = %q", observed.Fingerprint)
	}
	if got := authCallbacks.Load(); got != 0 {
		t.Fatalf("authentication callbacks invoked %d times", got)
	}
	<-serverDone
}

func TestValidateConnectionRequiresConfirmationAndPinsSecondHandshake(t *testing.T) {
	first := observedHostKey(newTestHostKey(t))
	changed := observedHostKey(newTestHostKey(t))
	probe := func(string, int, time.Duration) (ObservedHostKey, error) { return first, nil }

	if _, err := validateConnection("host", 22, "root", "secret", "", "", "", "", probe, func(string) error {
		t.Fatal("authentication attempted before confirmation")
		return nil
	}); !errors.Is(err, ErrHostKeyMissing) {
		t.Fatalf("unconfirmed error = %v", err)
	}

	authenticate := func(accepted string) error {
		callback, err := strictHostKeyCallback(accepted)
		if err != nil {
			return err
		}
		actual, err := parseTrustedHostKey(changed.AuthorizedKey)
		if err != nil {
			return err
		}
		return callback("host", nil, actual)
	}
	if _, err := validateConnection(
		"host", 22, "root", "secret", "", "", "", first.AuthorizedKey,
		probe, authenticate,
	); !errors.Is(err, ErrHostKeyChangedDuringValidation) {
		t.Fatalf("changed second handshake error = %v", err)
	}
}

func TestVerifyObservedHostKeyUsesPublicKeyBytes(t *testing.T) {
	observed := observedHostKey(newTestHostKey(t))
	withComment := observed.AuthorizedKey + " server-comment"
	accepted, err := verifyObservedHostKey(withComment, "", observed)
	if err != nil {
		t.Fatal(err)
	}
	if accepted != observed.AuthorizedKey {
		t.Fatalf("accepted host key = %q", accepted)
	}
	if !HostKeysEqual(withComment, observed.AuthorizedKey) {
		t.Fatal("equivalent host keys were treated as different")
	}
}

func TestVerifyObservedHostKeyAllowsRepairingMalformedStoredPin(t *testing.T) {
	observed := observedHostKey(newTestHostKey(t))
	if _, err := verifyObservedHostKey("not-a-public-key", "", observed); !errors.Is(err, ErrHostKeyMismatch) {
		t.Fatalf("malformed stored pin without confirmation error = %v", err)
	}
	accepted, err := verifyObservedHostKey("not-a-public-key", observed.AuthorizedKey, observed)
	if err != nil {
		t.Fatalf("confirmed replacement error = %v", err)
	}
	if accepted != observed.AuthorizedKey {
		t.Fatalf("accepted host key = %q", accepted)
	}
}

func TestPermanentHostKeyErrorsIncludeValidationSwitch(t *testing.T) {
	for _, err := range []error{
		ErrHostKeyMissing,
		ErrHostKeyMismatch,
		ErrHostKeyInvalid,
		ErrHostKeyChangedDuringValidation,
	} {
		if !IsPermanentHostKeyError(err) {
			t.Fatalf("%v was not classified as a permanent host key error", err)
		}
	}
}

func newTestSigner(t *testing.T) gossh.Signer {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func newTestHostKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	return newTestSigner(t).PublicKey()
}

func newTestPrivateKeyAndCertificate(t *testing.T) (string, string) {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	privateKeyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	}))

	signer, err := gossh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("NewSignerFromKey() error = %v", err)
	}

	_, caPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	caSigner, err := gossh.NewSignerFromKey(caPrivateKey)
	if err != nil {
		t.Fatalf("NewSignerFromKey() error = %v", err)
	}

	certificate := &gossh.Certificate{
		Key:             signer.PublicKey(),
		CertType:        gossh.UserCert,
		KeyId:           "test-cert",
		ValidPrincipals: []string{"root"},
		ValidAfter:      0,
		ValidBefore:     gossh.CertTimeInfinity,
	}
	if err = certificate.SignCert(rand.Reader, caSigner); err != nil {
		t.Fatalf("SignCert() error = %v", err)
	}

	return privateKeyPEM, strings.TrimSpace(string(gossh.MarshalAuthorizedKey(certificate)))
}
