package encrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"testing"
)

func TestEncryptDecryptAuthenticatedEnvelope(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	context := KeyContentContext(17)
	plaintext := []byte("private key material")

	first, err := Encrypt(plaintext, key, context)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encrypt(plaintext, key, context)
	if err != nil {
		t.Fatal(err)
	}
	if !IsCurrentCiphertext(first) {
		t.Fatal("new ciphertext does not use the current envelope")
	}
	if bytes.Equal(first, second) {
		t.Fatal("encrypting the same plaintext reused a nonce")
	}
	got, err := Decrypt(first, key, context)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("plaintext = %q, want %q", got, plaintext)
	}
}

func TestDecryptRejectsTamperingWrongKeyAndWrongRecord(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	ciphertext, err := Encrypt([]byte("secret"), key, SSHPasswordContext(9))
	if err != nil {
		t.Fatal(err)
	}

	tampered := append([]byte(nil), ciphertext...)
	tampered[len(tampered)-1] ^= 1
	for name, test := range map[string]struct {
		ciphertext []byte
		key        []byte
		context    string
	}{
		"tampered":     {ciphertext: tampered, key: key, context: SSHPasswordContext(9)},
		"wrong key":    {ciphertext: ciphertext, key: bytes.Repeat([]byte{0x24}, 32), context: SSHPasswordContext(9)},
		"wrong record": {ciphertext: ciphertext, key: key, context: SSHPasswordContext(10)},
		"wrong field":  {ciphertext: ciphertext, key: key, context: KeyPasswordContext(9)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decrypt(test.ciphertext, test.key, test.context); err == nil {
				t.Fatal("Decrypt() accepted unauthenticated ciphertext")
			}
		})
	}
}

func TestDecryptLegacyCBCIsRestrictedToMigrationPath(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	legacy := encryptLegacyCBCForTest(t, []byte("legacy credential"), key)
	original := append([]byte(nil), legacy...)

	if _, err := Decrypt(legacy, key, SSHPasswordContext(1)); err == nil {
		t.Fatal("runtime Decrypt() accepted legacy CBC ciphertext")
	}
	got, err := DecryptLegacy(legacy, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "legacy credential" {
		t.Fatalf("plaintext = %q", got)
	}
	if !bytes.Equal(legacy, original) {
		t.Fatal("legacy decryption mutated the ciphertext input")
	}
}

func TestDecryptRejectsUnknownEnvelopeVersion(t *testing.T) {
	ciphertext := append(append([]byte(nil), envelopeMagic...), byte(99))
	ciphertext = append(ciphertext, bytes.Repeat([]byte{0}, 64)...)
	if _, err := Decrypt(ciphertext, bytes.Repeat([]byte{0x42}, 32), "context"); err == nil {
		t.Fatal("Decrypt() accepted an unknown envelope version")
	}
}

func encryptLegacyCBCForTest(t *testing.T, plaintext, key []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := append(append([]byte(nil), plaintext...), bytes.Repeat([]byte{byte(padding)}, padding)...)
	ciphertext := make([]byte, aes.BlockSize+len(padded))
	if _, err := io.ReadFull(rand.Reader, ciphertext[:aes.BlockSize]); err != nil {
		t.Fatal(err)
	}
	cipher.NewCBCEncrypter(block, ciphertext[:aes.BlockSize]).CryptBlocks(ciphertext[aes.BlockSize:], padded)
	return ciphertext
}
