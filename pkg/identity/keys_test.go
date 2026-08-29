package identity

import (
	"bytes"
	"crypto/ed25519"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func TestParseEd25519PrivateKeyPEM(t *testing.T) {
	seed := bytes.Repeat([]byte{0x42}, ed25519.SeedSize)
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: seed})
	key, err := ParseEd25519PrivateKeyPEM(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key, ed25519.NewKeyFromSeed(seed)) {
		t.Fatal("parsed private key does not match seed")
	}
}

func TestParseEd25519PrivateKeyPEMRejectsMalformedInput(t *testing.T) {
	tests := map[string][]byte{
		"not PEM":       []byte("not PEM"),
		"wrong type":    pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: make([]byte, ed25519.SeedSize)}),
		"short seed":    pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("short")}),
		"trailing data": append(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: make([]byte, ed25519.SeedSize)}), []byte("unexpected")...),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseEd25519PrivateKeyPEM(data); err == nil {
				t.Fatal("malformed key was accepted")
			}
		})
	}
}

func TestParseEd25519PublicKeyPEMRejectsMalformedInput(t *testing.T) {
	tests := map[string][]byte{
		"not PEM":       []byte("not PEM"),
		"wrong type":    pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: make([]byte, ed25519.PublicKeySize)}),
		"short key":     pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("short")}),
		"trailing data": append(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: make([]byte, ed25519.PublicKeySize)}), []byte("unexpected")...),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseEd25519PublicKeyPEM(data); err == nil {
				t.Fatal("malformed key was accepted")
			}
		})
	}
}

func TestSignHeadersRejectsInvalidPrivateKeyLength(t *testing.T) {
	if _, err := SignHeaders(ed25519.PrivateKey("short"), "agent", 1, "nonce"); err == nil || !strings.Contains(err.Error(), "length") {
		t.Fatalf("SignHeaders() error = %v, want length error", err)
	}
}

func TestVerifySignedHeadersRejectsInvalidPublicKeyLength(t *testing.T) {
	if err := VerifySignedHeaders(ed25519.PublicKey("short"), "agent", "1", "nonce", "signature", time.Unix(1, 0)); err == nil || !strings.Contains(err.Error(), "public key length") {
		t.Fatalf("VerifySignedHeaders() error = %v, want public key length error", err)
	}
}

func TestEd25519PublicKeyEncodingAndFingerprint(t *testing.T) {
	_, publicPEM, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ParseEd25519PublicKeyPEM([]byte(publicPEM))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeEd25519PublicKeyPEM(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != publicPEM {
		t.Fatal("public key encoding was not canonical")
	}
	fingerprint, err := Ed25519Fingerprint(publicKey)
	if err != nil || !strings.HasPrefix(fingerprint, "SHA256:") {
		t.Fatalf("fingerprint = %q, %v", fingerprint, err)
	}
}
