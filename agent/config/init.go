package config

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"mosona-manager/agent/runtime"
	"mosona-manager/pkg/identity"
	"os"
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
	PrivateKey, err = identity.ParseEd25519PrivateKeyPEM(data)
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}
	return nil
}

// CreatePrivateKey atomically reserves the agent identity path and never overwrites an existing identity.
func CreatePrivateKey(data []byte) error {
	return createSecureFile(filepath.Join(runtime.InstallDir, "private_key.pem"), data, 0o600)
}

func LoadOrCreateActivePrivateKey() error {
	if err := LoadPrivateKey(); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	privateKey, _, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		return fmt.Errorf("generate active identity: %w", err)
	}
	if err = CreatePrivateKey([]byte(privateKey)); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create active identity: %w", err)
	}
	return LoadPrivateKey()
}

func LoadPublicKey() error {
	PublicKey = nil
	data, err := readSecureFile(filepath.Join(runtime.InstallDir, "public_key.pem"), 0o600)
	if err != nil {
		return fmt.Errorf("read public key file: %w", err)
	}
	PublicKey, err = identity.ParseEd25519PublicKeyPEM(data)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}
	return nil
}
