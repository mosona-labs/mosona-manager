package install

import (
	"fmt"
	"mosona-manager/agent/config"
	"os"
	"path/filepath"
)

func Active(uid, publicKey, host string, port int, noMonitor, noTerminal bool) error {
	fmt.Println("Initializing agent in active mode...")
	fmt.Printf("Will be listening on %s:%d\n", host, port)

	if err := prepareInstallDir(); err != nil {
		return err
	}
	if err := initializeActiveIdentity(); err != nil {
		return err
	}

	// Save public key
	publicKeyFile, err := os.Create(
		filepath.Join(installDir(), "public_key.pem"),
	)
	if err != nil {
		return err
	}
	defer func() {
		_ = publicKeyFile.Close()
	}()
	if _, err = publicKeyFile.WriteString(publicKey); err != nil {
		return err
	}

	// Save config
	conf := config.Config{
		Mode: "active",

		NoMonitor:  noMonitor,
		NoTerminal: noTerminal,

		Uid:  uid,
		Host: host,
		Port: port,
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

func initializeActiveIdentity() error {
	if err := config.LoadOrCreateActivePrivateKey(); err != nil {
		return fmt.Errorf("initialize Active Agent identity: %w", err)
	}
	return nil
}
