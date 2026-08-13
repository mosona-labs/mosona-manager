package encrypt

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	KeyPathEnv    = "MOSONA_ENCRYPTION_KEY_PATH"
	legacyKeyPath = "cfg/key"
)

var (
	Key           []byte // Password encryption key
	ErrKeyMissing = errors.New("encryption key is missing while encrypted credentials exist")
	userConfigDir = os.UserConfigDir
)

// Initialize loads the master key, creating it only for an installation without encrypted credentials.
func Initialize(encryptedCredentialsExist bool) (string, error) {
	Key = nil
	keyPath, err := resolveKeyPath(encryptedCredentialsExist)
	if err != nil {
		return "", err
	}
	key, err := initializeKey(keyPath, encryptedCredentialsExist)
	if err != nil {
		return keyPath, err
	}
	Key = key
	return keyPath, nil
}

func resolveKeyPath(encryptedCredentialsExist bool) (string, error) {
	configured := strings.TrimSpace(os.Getenv(KeyPathEnv))
	if configured != "" {
		if !filepath.IsAbs(configured) {
			return "", fmt.Errorf("%s must be an absolute path", KeyPathEnv)
		}
		return filepath.Clean(configured), nil
	}

	legacyPath, err := filepath.Abs(legacyKeyPath)
	if err != nil {
		return "", fmt.Errorf("resolve legacy encryption key path: %w", err)
	}
	if _, err := os.Lstat(legacyPath); err == nil {
		return legacyPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect legacy encryption key path: %w", err)
	}

	configDir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve encryption key directory: %w", err)
	}
	stablePath := filepath.Join(configDir, "mosona-manager", "key")
	if _, err := os.Lstat(stablePath); err == nil {
		return stablePath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect encryption key path: %w", err)
	}
	if !encryptedCredentialsExist {
		return stablePath, nil
	}
	return legacyPath, nil
}

func initializeKey(keyPath string, encryptedCredentialsExist bool) ([]byte, error) {
	info, err := os.Lstat(keyPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, errors.New("encryption key must be a regular file")
		}
		if err := secureDirectory(filepath.Dir(keyPath)); err != nil {
			return nil, err
		}
		return loadExistingKey(keyPath)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect encryption key: %w", err)
	}
	if encryptedCredentialsExist {
		return nil, fmt.Errorf("%w: restore %s from backup or configure %s", ErrKeyMissing, keyPath, KeyPathEnv)
	}
	if err := secureDirectory(filepath.Dir(keyPath)); err != nil {
		return nil, err
	}
	return createKey(keyPath)
}

func secureDirectory(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create encryption key directory: %w", err)
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return fmt.Errorf("inspect encryption key directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("encryption key directory must be a real directory")
	}
	if err := validateOwner(dir, info); err != nil {
		return err
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("secure encryption key directory: %w", err)
		}
	}
	return nil
}

func loadExistingKey(keyPath string) ([]byte, error) {
	file, err := openKeyFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("open encryption key: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect encryption key: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("encryption key must be a regular file")
	}
	if err := validateOwner(keyPath, info); err != nil {
		return nil, err
	}
	if info.Mode().Perm() != 0o600 {
		if err := file.Chmod(0o600); err != nil {
			return nil, fmt.Errorf("secure encryption key permissions: %w", err)
		}
	}

	key, err := io.ReadAll(io.LimitReader(file, 33))
	if err != nil {
		return nil, fmt.Errorf("read encryption key: %w", err)
	}
	if err := validateKeyLength(key); err != nil {
		return nil, fmt.Errorf("invalid encryption key file: %w", err)
	}
	return key, nil
}

func createKey(keyPath string) ([]byte, error) {
	key, err := GenerateKey(32)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(keyPath)
	file, err := os.CreateTemp(dir, ".key-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary encryption key: %w", err)
	}
	temporaryPath := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := file.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("secure temporary encryption key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		return nil, fmt.Errorf("write encryption key: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("sync encryption key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close encryption key: %w", err)
	}
	if err := os.Link(temporaryPath, keyPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return loadExistingKey(keyPath)
		}
		return nil, fmt.Errorf("publish encryption key: %w", err)
	}
	if err := syncDirectory(dir); err != nil {
		return nil, err
	}
	return key, nil
}

func validateKeyLength(key []byte) error {
	switch len(key) {
	case 16, 24, 32:
		return nil
	default:
		return fmt.Errorf("length is %d bytes; expected 16, 24, or 32", len(key))
	}
}

func syncDirectory(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open encryption key directory: %w", err)
	}
	defer func() { _ = handle.Close() }()
	if err := handle.Sync(); err != nil && !ignoreUnsupportedDirectorySync(err) {
		return fmt.Errorf("sync encryption key directory: %w", err)
	}
	return nil
}
