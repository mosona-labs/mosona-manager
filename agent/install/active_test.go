package install

import (
	"bytes"
	"crypto/ed25519"
	"mosona-manager/agent/config"
	agentRuntime "mosona-manager/agent/runtime"
	"mosona-manager/pkg/identity"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useActiveInstallTestDir(t *testing.T) string {
	t.Helper()
	oldInstallDir := agentRuntime.InstallDir
	oldPrivateKey := config.PrivateKey
	agentRuntime.InstallDir = filepath.Join(t.TempDir(), "agent")
	t.Cleanup(func() {
		agentRuntime.InstallDir = oldInstallDir
		config.PrivateKey = oldPrivateKey
	})
	if err := os.MkdirAll(agentRuntime.InstallDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return agentRuntime.InstallDir
}

func TestInitializeActiveIdentityReusesExistingKey(t *testing.T) {
	dir := useActiveInstallTestDir(t)
	privateKeyPEM, _, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "private_key.pem")
	if err = os.WriteFile(keyPath, []byte(privateKeyPEM), 0o600); err != nil {
		t.Fatal(err)
	}

	if err = initializeActiveIdentity(); err != nil {
		t.Fatal(err)
	}
	parsed, err := identity.ParseEd25519PrivateKeyPEM([]byte(privateKeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(config.PrivateKey, ed25519.PrivateKey(parsed)) {
		t.Fatal("existing Active Agent identity was replaced")
	}
}

func TestInitializeActiveIdentityReportsInvalidExistingKey(t *testing.T) {
	dir := useActiveInstallTestDir(t)
	if err := os.WriteFile(filepath.Join(dir, "private_key.pem"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := initializeActiveIdentity()
	if err == nil || !strings.Contains(err.Error(), "initialize Active Agent identity") {
		t.Fatalf("initializeActiveIdentity() error = %v, want contextual error", err)
	}
}
