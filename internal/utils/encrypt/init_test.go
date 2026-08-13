package encrypt

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestInitializeCreatesSecureKeyForFreshInstall(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "state", "key")
	t.Setenv(KeyPathEnv, keyPath)
	oldKey := Key
	t.Cleanup(func() { Key = oldKey })

	resolved, err := Initialize(false)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != keyPath || len(Key) != 32 {
		t.Fatalf("resolved = %q, key length = %d", resolved, len(Key))
	}
	assertMode(t, filepath.Dir(keyPath), 0o700)
	assertMode(t, keyPath, 0o600)
	onDisk, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, Key) {
		t.Fatal("in-memory key differs from persisted key")
	}
}

func TestInitializeLoadsLegacyKeyAndRepairsPermissions(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "cfg", "key")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("0123456789abcdef0123456789abcdef")
	if err := os.WriteFile(keyPath, want, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(keyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := initializeKey(keyPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("key = %q", got)
	}
	assertMode(t, filepath.Dir(keyPath), 0o700)
	assertMode(t, keyPath, 0o600)
}

func TestInitializeDiscoversLegacyRelativeKey(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv(KeyPathEnv, "")
	keyPath := filepath.Join(root, "cfg", "key")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	want := []byte("0123456789abcdef0123456789abcdef")
	if err := os.WriteFile(keyPath, want, 0o600); err != nil {
		t.Fatal(err)
	}
	oldKey := Key
	t.Cleanup(func() { Key = oldKey })

	resolved, err := Initialize(true)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != keyPath || !bytes.Equal(Key, want) {
		t.Fatalf("resolved = %q, key = %q", resolved, Key)
	}
}

func TestInitializeReusesStableDefaultPathAfterCredentialsAreStored(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv(KeyPathEnv, "")
	configRoot := filepath.Join(root, "config")
	oldKey := Key
	oldUserConfigDir := userConfigDir
	userConfigDir = func() (string, error) { return configRoot, nil }
	t.Cleanup(func() {
		Key = oldKey
		userConfigDir = oldUserConfigDir
	})

	createdAt, err := Initialize(false)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte(nil), Key...)
	Key = nil
	loadedFrom, err := Initialize(true)
	if err != nil {
		t.Fatal(err)
	}
	if createdAt != loadedFrom || createdAt != filepath.Join(configRoot, "mosona-manager", "key") {
		t.Fatalf("created at %q, loaded from %q", createdAt, loadedFrom)
	}
	if !bytes.Equal(Key, want) {
		t.Fatal("restarted installation loaded a different key")
	}
}

func TestConcurrentFreshInitializationUsesSingleKey(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "cfg", "key")
	const workers = 8
	keys := make(chan []byte, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			key, err := initializeKey(keyPath, false)
			keys <- key
			errs <- err
		}()
	}
	wait.Wait()
	close(keys)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	for key := range keys {
		if !bytes.Equal(key, want) {
			t.Fatal("concurrent initializer returned a different key")
		}
	}
}

func TestInitializeFailsClosedWhenCredentialsExist(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "missing", "key")
	_, err := initializeKey(keyPath, true)
	if !errors.Is(err, ErrKeyMissing) {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Dir(keyPath)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("missing-key check created directory: %v", statErr)
	}
}

func TestInitializeRejectsInvalidKeyLength(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "cfg", "key")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("too-short"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := initializeKey(keyPath, false)
	if err == nil || !strings.Contains(err.Error(), "expected 16, 24, or 32") {
		t.Fatalf("error = %v", err)
	}
}

func TestInitializeRejectsSymlinkKey(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(root, "key")
	if err := os.Symlink(target, keyPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := initializeKey(keyPath, false)
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("error = %v", err)
	}
}

func TestInitializeRejectsRelativeConfiguredPath(t *testing.T) {
	t.Setenv(KeyPathEnv, "cfg/key")
	oldKey := Key
	Key = []byte("stale")
	t.Cleanup(func() { Key = oldKey })

	_, err := Initialize(false)
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("error = %v", err)
	}
	if Key != nil {
		t.Fatalf("key retained after initialization failure: %q", Key)
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
