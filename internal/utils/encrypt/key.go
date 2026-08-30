package encrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strconv"
)

const (
	envelopeVersion byte = 1
)

var envelopeMagic = []byte("MSNENC01")

// EnvelopeMagic returns the prefix shared by every versioned ciphertext
// envelope. Callers outside the package use it to recognize envelopes without
// decrypting them (for example the database probe in HasEncryptedCredentials),
// so detection can never drift from Decrypt.
func EnvelopeMagic() string {
	return string(envelopeMagic)
}

func KeyContentContext(keyID int64) string {
	return "keys/" + strconv.FormatInt(keyID, 10) + "/content"
}

func KeyPasswordContext(keyID int64) string {
	return "keys/" + strconv.FormatInt(keyID, 10) + "/password"
}

func SSHPasswordContext(serverID int64) string {
	return "ssh/" + strconv.FormatInt(serverID, 10) + "/password"
}

func AgentPrivateKeyContext(serverID int64) string {
	return "agents/" + strconv.FormatInt(serverID, 10) + "/private_key"
}

func GenerateKey(length int) ([]byte, error) {
	if length != 16 && length != 24 && length != 32 {
		return nil, errors.New("invalid key length: must be 16, 24, or 32")
	}
	key := make([]byte, length)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// Encrypt seals plaintext in a versioned AES-GCM envelope. The context binds
// ciphertext to its database record and field.
func Encrypt(plaintext []byte, key []byte, context string) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	header := append(append([]byte(nil), envelopeMagic...), envelopeVersion)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	envelope := make([]byte, 0, len(header)+len(nonce)+len(plaintext)+gcm.Overhead())
	envelope = append(envelope, header...)
	envelope = append(envelope, nonce...)
	return gcm.Seal(envelope, nonce, plaintext, associatedData(header, context)), nil
}

// Decrypt opens only authenticated envelopes. Legacy CBC compatibility is
// deliberately restricted to the startup migration to prevent downgrades.
func Decrypt(ciphertext []byte, key []byte, context string) ([]byte, error) {
	if !IsVersionedCiphertext(ciphertext) {
		return nil, errors.New("unsupported legacy ciphertext outside migration")
	}
	return decryptEnvelope(ciphertext, key, context)
}

// DecryptLegacy opens the historical AES-CBC format. It must only be used by
// the startup migration, before application handlers and workers are started.
func DecryptLegacy(ciphertext []byte, key []byte) ([]byte, error) {
	return decryptLegacyCBC(ciphertext, key)
}

func IsVersionedCiphertext(ciphertext []byte) bool {
	return bytes.HasPrefix(ciphertext, envelopeMagic)
}

func IsCurrentCiphertext(ciphertext []byte) bool {
	return len(ciphertext) > len(envelopeMagic) &&
		bytes.Equal(ciphertext[:len(envelopeMagic)], envelopeMagic) &&
		ciphertext[len(envelopeMagic)] == envelopeVersion
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func decryptEnvelope(envelope []byte, key []byte, context string) ([]byte, error) {
	headerLen := len(envelopeMagic) + 1
	if !IsVersionedCiphertext(envelope) || len(envelope) < headerLen {
		return nil, errors.New("encrypted envelope is truncated")
	}
	version := envelope[len(envelopeMagic)]
	if version != envelopeVersion {
		return nil, fmt.Errorf("unsupported encrypted envelope version %d", version)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(envelope) < headerLen+gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("encrypted envelope is truncated")
	}
	header := envelope[:headerLen]
	nonce := envelope[headerLen : headerLen+gcm.NonceSize()]
	sealed := envelope[headerLen+gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, sealed, associatedData(header, context))
	if err != nil {
		return nil, fmt.Errorf("authenticate encrypted data: %w", err)
	}
	return plaintext, nil
}

func associatedData(header []byte, context string) []byte {
	aad := make([]byte, 0, len(header)+1+len(context))
	aad = append(aad, header...)
	aad = append(aad, 0)
	return append(aad, context...)
}

func decryptLegacyCBC(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < aes.BlockSize {
		return nil, errors.New("ciphertext too short")
	}
	body := ciphertext[aes.BlockSize:]
	if len(body) == 0 || len(body)%aes.BlockSize != 0 {
		return nil, errors.New("invalid legacy ciphertext length")
	}
	plaintext := append([]byte(nil), body...)
	mode := cipher.NewCBCDecrypter(block, ciphertext[:aes.BlockSize])
	mode.CryptBlocks(plaintext, plaintext)
	return pkcs7UnPad(plaintext, aes.BlockSize)
}

func pkcs7UnPad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("data is empty")
	}
	padding := int(data[len(data)-1])
	if padding > blockSize || padding == 0 || padding > len(data) {
		return nil, errors.New("invalid padding")
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, errors.New("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}
