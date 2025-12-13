package encrypt

import (
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
)

var (
	Key []byte // Password encryption key
)

var configPath = getConfigPath()

func getConfigPath() string {
	if runtime.GOOS == "windows" {
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = "C:\\ProgramData"
		}
		return filepath.Join(programData, "MosonaManager")
	}
	return "/etc/mosona-manager/"
}

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
