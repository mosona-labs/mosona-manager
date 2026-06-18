package utils

import (
	"crypto/ed25519"
	"encoding/pem"
	"errors"
)

func ParseAgentEd25519PublicKeyPEM(publicKey string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKey))
	if block == nil || len(block.Bytes) != ed25519.PublicKeySize {
		return nil, errors.New("invalid public key")
	}
	return ed25519.PublicKey(block.Bytes), nil
}
