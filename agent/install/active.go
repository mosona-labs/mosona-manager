package install

import (
	"fmt"
	"mosona-manager/agent/config"
	"mosona-manager/agent/runtime"
	"os"
	"path"
)

func Active(uid, publicKey, host string, port int, noMonitor, noTerminal bool) error {
	fmt.Println("Initializing agent in active mode...")
	fmt.Printf("Will be listening on %s:%d\n", host, port)

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

	// Save public key
	publicKeyFile, err := os.Create(
		path.Join(runtime.InstallDir, "public_key.pem"),
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
