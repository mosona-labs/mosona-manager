package config

import (
	"bytes"
	"crypto/ed25519"
	"encoding/pem"
	"errors"
	"mosona-manager/agent/runtime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func useTestInstallDir(t *testing.T) string {
	t.Helper()
	oldInstallDir := runtime.InstallDir
	runtime.InstallDir = filepath.Join(t.TempDir(), "agent")
	t.Cleanup(func() { runtime.InstallDir = oldInstallDir })
	if err := os.MkdirAll(runtime.InstallDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return runtime.InstallDir
}

func TestCreatePrivateKeyUsesExclusiveSecureFile(t *testing.T) {
	dir := useTestInstallDir(t)
	first := []byte("first private key")
	if err := CreatePrivateKey(first); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "private_key.pem")
	assertMode(t, keyPath, 0o600)

	err := CreatePrivateKey([]byte("replacement"))
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("second create error = %v, want os.ErrExist", err)
	}
	got, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, first) {
		t.Fatalf("existing private key was overwritten: %q", got)
	}
}

func TestConcurrentPrivateKeyCreationHasSingleWinner(t *testing.T) {
	dir := useTestInstallDir(t)
	const workers = 8
	var successes atomic.Int32
	errCh := make(chan error, workers)
	contents := make([][]byte, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for i := range workers {
		contents[i] = []byte(strings.Repeat(string(rune('a'+i)), 64))
		go func(data []byte) {
			defer wait.Done()
			err := CreatePrivateKey(data)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, os.ErrExist) {
				errCh <- err
			}
		}(contents[i])
	}
	wait.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful creators = %d, want 1", got)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "private_key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, content := range contents {
		if bytes.Equal(onDisk, content) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("key file contains incomplete or unexpected data: %q", onDisk)
	}
}

func TestLoadRepairsLegacyOwnedModes(t *testing.T) {
	dir := useTestInstallDir(t)
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"mode":"passive"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configPath, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Load(); err != nil {
		t.Fatal(err)
	}
	assertMode(t, dir, 0o700)
	assertMode(t, configPath, 0o600)
}

func TestLoadPrivateKeyRepairsModeAndValidatesSeed(t *testing.T) {
	dir := useTestInstallDir(t)
	seed := bytes.Repeat([]byte{0x42}, ed25519.SeedSize)
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: seed})
	keyPath := filepath.Join(dir, "private_key.pem")
	if err := os.WriteFile(keyPath, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	oldPrivateKey := PrivateKey
	t.Cleanup(func() { PrivateKey = oldPrivateKey })

	if err := LoadPrivateKey(); err != nil {
		t.Fatal(err)
	}
	assertMode(t, keyPath, 0o600)
	if want := ed25519.NewKeyFromSeed(seed); !bytes.Equal(PrivateKey, want) {
		t.Fatal("loaded private key differs from encoded seed")
	}

	invalid := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("short")})
	if err := os.WriteFile(keyPath, invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadPrivateKey(); err == nil || !strings.Contains(err.Error(), "seed length") {
		t.Fatalf("invalid seed error = %v", err)
	}
	if PrivateKey != nil {
		t.Fatal("invalid private key left stale key material loaded")
	}
}

func TestLoadPublicKeyValidatesLengthAndClearsStaleKey(t *testing.T) {
	dir := useTestInstallDir(t)
	keyPath := filepath.Join(dir, "public_key.pem")
	invalid := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("short")})
	if err := os.WriteFile(keyPath, invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	oldPublicKey := PublicKey
	PublicKey = ed25519.PublicKey("stale")
	t.Cleanup(func() { PublicKey = oldPublicKey })

	if err := LoadPublicKey(); err == nil || !strings.Contains(err.Error(), "public key length") {
		t.Fatalf("invalid public key error = %v", err)
	}
	if PublicKey != nil {
		t.Fatal("invalid public key left stale key material loaded")
	}
}

func TestLoadPrivateKeyRejectsSymlink(t *testing.T) {
	dir := useTestInstallDir(t)
	target := filepath.Join(t.TempDir(), "private_key.pem")
	seed := bytes.Repeat([]byte{0x24}, ed25519.SeedSize)
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: seed})
	if err := os.WriteFile(target, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "private_key.pem")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := LoadPrivateKey(); err == nil {
		t.Fatal("symlinked private key was accepted")
	}
}

func TestLoadOrCreateActivePrivateKeyPersistsIdentity(t *testing.T) {
	dir := useTestInstallDir(t)
	oldPrivateKey := PrivateKey
	t.Cleanup(func() { PrivateKey = oldPrivateKey })

	if err := LoadOrCreateActivePrivateKey(); err != nil {
		t.Fatal(err)
	}
	first := append(ed25519.PrivateKey(nil), PrivateKey...)
	assertMode(t, filepath.Join(dir, "private_key.pem"), 0o600)
	PrivateKey = nil
	if err := LoadOrCreateActivePrivateKey(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, PrivateKey) {
		t.Fatal("active identity changed after reload")
	}
}

func TestLoadRejectsSymlinkInstallDirectory(t *testing.T) {
	oldInstallDir := runtime.InstallDir
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "config.json"), []byte(`{"mode":"passive"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime.InstallDir = filepath.Join(root, "agent")
	t.Cleanup(func() { runtime.InstallDir = oldInstallDir })
	if err := os.Symlink(target, runtime.InstallDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := Load(); err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("symlink install directory error = %v", err)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}
