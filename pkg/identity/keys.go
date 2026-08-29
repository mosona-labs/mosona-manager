package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"fmt"
)

func ParseEd25519PrivateKeyPEM(data []byte) (ed25519.PrivateKey, error) {
	block, rest := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("decode Ed25519 private key PEM block")
	}
	if block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("invalid Ed25519 private key type: %s", block.Type)
	}
	if len(block.Bytes) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid Ed25519 private key seed length: got %d, want %d", len(block.Bytes), ed25519.SeedSize)
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return nil, fmt.Errorf("unexpected data after Ed25519 private key PEM block")
	}
	return ed25519.NewKeyFromSeed(block.Bytes), nil
}

func ParseEd25519PublicKeyPEM(data []byte) (ed25519.PublicKey, error) {
	block, rest := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("decode Ed25519 public key PEM block")
	}
	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("invalid Ed25519 public key type: %s", block.Type)
	}
	if len(block.Bytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid Ed25519 public key length: got %d, want %d", len(block.Bytes), ed25519.PublicKeySize)
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return nil, fmt.Errorf("unexpected data after Ed25519 public key PEM block")
	}
	return ed25519.PublicKey(block.Bytes), nil
}

func GenerateEd25519KeyPair() (privateKeyStr, publicKeyStr string, err error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	privateKeyBytes := privateKey.Seed()
	privateKeyBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	privateKeyStr = string(pem.EncodeToMemory(privateKeyBlock))

	publicKeyBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKey,
	}
	publicKeyStr = string(pem.EncodeToMemory(publicKeyBlock))

	return privateKeyStr, publicKeyStr, nil
}

func EncodeEd25519PublicKeyPEM(publicKey ed25519.PublicKey) (string, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid Ed25519 public key length: got %d, want %d", len(publicKey), ed25519.PublicKeySize)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicKey})), nil
}

func Ed25519Fingerprint(publicKey ed25519.PublicKey) (string, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid Ed25519 public key length: got %d, want %d", len(publicKey), ed25519.PublicKeySize)
	}
	digest := sha256.Sum256(publicKey)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:]), nil
}

func SignHeaders(privateKey ed25519.PrivateKey, uuid string, ts int64, nonce string) (string, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("invalid Ed25519 private key length: got %d, want %d", len(privateKey), ed25519.PrivateKeySize)
	}

	msg := fmt.Sprintf("%s\n%d\n%s", uuid, ts, nonce)

	sig := ed25519.Sign(privateKey, []byte(msg))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	return sigB64, nil
}
