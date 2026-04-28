package install

import (
	"fmt"
	"mosona-manager/agent/config"
	"mosona-manager/agent/runtime"
	"mosona-manager/pkg/identity"
	"os"
	"path/filepath"
)

func Passive(hub, enrollKey string, noMonitor, noTerminal bool, ipPreference string) error {
	fmt.Println("Initializing agent in passive mode...")

	if _, err := os.Stat(runtime.InstallDir); !os.IsNotExist(err) {
		fmt.Print("Do you want to reinstall agent? (y/N): ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			return fmt.Errorf("installation cancelled by user")
		}
		if err = os.RemoveAll(runtime.InstallDir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(runtime.InstallDir, 0755); err != nil {
		return err
	}

	privateKey, publicKey, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		return err
	}

	agentUID, err := EnrollPassive(hub, enrollKey, publicKey, ipPreference)
	if err != nil {
		return err
	}

	// Save private key
	privateKeyFile, err := os.Create(
		filepath.Join(runtime.InstallDir, "private_key.pem"),
	)
	if err != nil {
		return err
	}
	defer func() {
		_ = privateKeyFile.Close()
	}()
	if _, err = privateKeyFile.WriteString(privateKey); err != nil {
		return err
	}

	// Save config
	conf := config.Config{
		Mode: "passive",

		NoMonitor:  noMonitor,
		NoTerminal: noTerminal,

		IPPreference: ipPreference,
		Hub:          hub,
		UUID:         agentUID,
	}
	if err = conf.Save(); err != nil {
		return err
	}

	// Copy self binary
	if err = copyBinaryToInstallDir(); err != nil {
		return err
	}

	// Install Service
	if err = installService(); err != nil {
		return err
	}

	return nil
}
