package update

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func fileSHA256Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func parseChecksumFile(data []byte) (string, error) {
	line := strings.TrimSpace(string(data))
	if line == "" {
		return "", fmt.Errorf("empty checksum file")
	}
	fields := strings.Fields(line)
	if len(fields) < 1 {
		return "", fmt.Errorf("invalid checksum file")
	}
	sum := strings.ToLower(fields[0])
	if len(sum) != 64 {
		return "", fmt.Errorf("invalid sha256 length")
	}
	for _, c := range sum {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", fmt.Errorf("invalid sha256 character")
		}
	}
	return sum, nil
}

func verifyFileSHA256(path, wantHex string) error {
	got, err := fileSHA256Hex(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, wantHex) {
		return fmt.Errorf("sha256 mismatch")
	}
	return nil
}
