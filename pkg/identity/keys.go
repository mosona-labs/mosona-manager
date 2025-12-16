package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"fmt"
)

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

func SignHeaders(privateKey ed25519.PrivateKey, uuid string, ts int64, nonce string) (string, error) {
	if len(privateKey) == 0 {
		return "", fmt.Errorf("missing private key")
	}

	msg := fmt.Sprintf("%s\n%d\n%s", uuid, ts, nonce)

	sig := ed25519.Sign(privateKey, []byte(msg))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	return sigB64, nil
}
