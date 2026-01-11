package encrypt

import (
	"log"
	"os"
	"path"
)

var (
	Key []byte // Password encryption key
)

var configPath = "cfg"

func init() {
	var err error

	// Init config directory
	if _, err = os.Stat(configPath); os.IsNotExist(err) {
		err = os.MkdirAll(configPath, 0700)
		if err != nil {
			log.Fatalf("Failed to create %s directory: %v\n", configPath, err)
		}
	}

	// Password Key
	Key, err = os.ReadFile(path.Join(configPath, "key"))
	if err != nil || Key == nil {
		Key, err = initKey()
		if err != nil {
			log.Fatalln("Failed to initialize encryption key:", err)
		}
	}
}
