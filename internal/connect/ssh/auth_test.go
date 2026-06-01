package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

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
