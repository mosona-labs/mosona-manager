package config

import (
	"crypto/ed25519"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"mosona-manager/agent/runtime"
	"os"
	"path"
)

var (
	Current Config

	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
)

func Load() error {
	file, err := os.ReadFile(path.Join(runtime.InstallDir, "config.json"))
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
	if err = os.WriteFile(path.Join(runtime.InstallDir, "config.json"), data, 0644); err != nil {
		return err
	}

	Current = conf

	return nil
}

func LoadPrivateKey() error {
	data, err := os.ReadFile(path.Join(runtime.InstallDir, "private_key.pem"))
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
	PrivateKey = ed25519.NewKeyFromSeed(block.Bytes)
	return nil
}

func LoadPublicKey() error {
	data, err := os.ReadFile(path.Join(runtime.InstallDir, "public_key.pem"))
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
