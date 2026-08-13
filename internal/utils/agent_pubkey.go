package utils

import (
	"crypto/ed25519"
	"mosona-manager/pkg/identity"
)

func ParseAgentEd25519PublicKeyPEM(publicKey string) (ed25519.PublicKey, error) {
	return identity.ParseEd25519PublicKeyPEM([]byte(publicKey))
}
