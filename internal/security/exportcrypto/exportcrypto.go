package exportcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	fileFormat       = "mosona-team-export-v1"
	kdfName          = "argon2id"
	minPasswordRunes = 8
	maxPasswordBytes = 1024

	argon2MemoryKiB  = 64 * 1024
	argon2Iterations = 3
	argon2Parallel   = 2
	saltLen          = 16
	keyLen           = 32
)

type EncryptedFile struct {
	Format     string `json:"format"`
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func EncryptJSON(password string, payload any) (*EncryptedFile, error) {
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	plain, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	salt := make([]byte, saltLen)
	if _, err = io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	key := argon2.IDKey([]byte(password), salt, argon2Iterations, argon2MemoryKiB, argon2Parallel, keyLen)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plain, nil)

	return &EncryptedFile{
		Format:     fileFormat,
		KDF:        kdfName,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(sealed),
	}, nil
}

func DecryptJSON(password string, file *EncryptedFile, out any) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	if file == nil {
		return errors.New("missing encrypted export payload")
	}
	if file.Format != fileFormat || file.KDF != kdfName {
		return fmt.Errorf("unsupported export format")
	}

	salt, err := base64.StdEncoding.DecodeString(file.Salt)
	if err != nil || len(salt) == 0 {
		return errors.New("invalid export salt")
	}
	nonce, err := base64.StdEncoding.DecodeString(file.Nonce)
	if err != nil || len(nonce) == 0 {
		return errors.New("invalid export nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(file.Ciphertext)
	if err != nil || len(ciphertext) == 0 {
		return errors.New("invalid export ciphertext")
	}

	key := argon2.IDKey([]byte(password), salt, argon2Iterations, argon2MemoryKiB, argon2Parallel, keyLen)
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(nonce) != gcm.NonceSize() {
		return errors.New("invalid export nonce length")
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return errors.New("export password incorrect or file corrupted")
	}
	if err = json.Unmarshal(plain, out); err != nil {
		return errors.New("decrypted export is not valid team data")
	}
	return nil
}

func validatePassword(password string) error {
	if password == "" {
		return errors.New("export password is required")
	}
	if len(password) > maxPasswordBytes {
		return errors.New("export password is too long")
	}
	n := 0
	for range password {
		n++
	}
	if n < minPasswordRunes {
		return fmt.Errorf("export password must be at least %d characters", minPasswordRunes)
	}
	return nil
}