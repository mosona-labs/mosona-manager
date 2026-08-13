package config

import (
	"crypto/ed25519"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"mosona-manager/agent/runtime"
	"path/filepath"
)

var (
	Current Config

	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
)

func Load() error {
	file, err := readSecureFile(filepath.Join(runtime.InstallDir, "config.json"), 0o600)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	if err = json.Unmarshal(file, &Current); err != nil {
		return fmt.Errorf("parse config file: %w", err)
	}
	return nil
}

func (conf Config) Save() error {
	data, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}
	if err = writeSecureFile(filepath.Join(runtime.InstallDir, "config.json"), data, 0o600); err != nil {
		return err
	}

	Current = conf

	return nil
}

func LoadPrivateKey() error {
	PrivateKey = nil
	data, err := readSecureFile(filepath.Join(runtime.InstallDir, "private_key.pem"), 0o600)
	if err != nil {
		return fmt.Errorf("read private key file: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("decode private key PEM block")
	}
	if block.Type != "PRIVATE KEY" {
		return fmt.Errorf("invalid private key type: %s", block.Type)
	}
	if len(block.Bytes) != ed25519.SeedSize {
		return fmt.Errorf("invalid private key seed length: got %d, want %d", len(block.Bytes), ed25519.SeedSize)
	}
	PrivateKey = ed25519.NewKeyFromSeed(block.Bytes)
	return nil
}

// CreatePrivateKey atomically reserves the passive-agent key path and never overwrites an existing identity.
func CreatePrivateKey(data []byte) error {
	return createSecureFile(filepath.Join(runtime.InstallDir, "private_key.pem"), data, 0o600)
}

func LoadPublicKey() error {
	data, err := readSecureFile(filepath.Join(runtime.InstallDir, "public_key.pem"), 0o600)
	if err != nil {
		return fmt.Errorf("read public key file: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("decode public key PEM block")
	}
	if block.Type != "PUBLIC KEY" {
		return fmt.Errorf("invalid public key type: %s", block.Type)
	}
	PublicKey = block.Bytes
	return nil
}
