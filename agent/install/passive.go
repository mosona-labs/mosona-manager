package install

import (
	"fmt"
	"mosona-manager/agent/config"
	"mosona-manager/pkg/identity"
)

func Passive(hub, enrollKey string, noMonitor, noTerminal bool, ipPreference string) error {
	fmt.Println("Initializing agent in passive mode...")

	if err := prepareInstallDir(); err != nil {
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

	if err := config.CreatePrivateKey([]byte(privateKey)); err != nil {
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
