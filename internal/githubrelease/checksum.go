package githubrelease

import (
	"fmt"
	"strings"
)

func ParseChecksumFile(data []byte) (string, error) {
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
