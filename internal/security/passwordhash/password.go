package passwordhash

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Prefix     = "$argon2id$"
	argon2Version    = 19
	argon2MemoryKiB  = 64 * 1024
	argon2Iterations = 3
	argon2Parallel   = 2
	argon2SaltLen    = 16
	argon2KeyLen     = 32
	legacyHexLen     = 64

	maxArgon2MemoryKiB  = 256 * 1024
	maxArgon2Iterations = 10
	maxArgon2Parallel   = 8
	maxArgon2SaltLen    = 64
	maxArgon2HashLen    = 64
)

func Hash(password string) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argon2Iterations, argon2MemoryKiB, argon2Parallel, argon2KeyLen)
	return encodePHC(salt, hash), nil
}

func Verify(password, storedHash, legacySalt, siteToken string) (ok bool, needsRehash bool, err error) {
	if IsArgon2idHash(storedHash) {
		ok, err = verifyArgon2id(password, storedHash)
		return ok, false, err
	}
	if IsLegacySHA256Hash(storedHash) {
		expected := legacySHA256Hex(password + legacySalt + siteToken)
		ok = subtle.ConstantTimeCompare([]byte(expected), []byte(storedHash)) == 1
		return ok, ok, nil
	}
	return false, false, nil
}

func IsLegacySHA256Hash(storedHash string) bool {
	if len(storedHash) != legacyHexLen {
		return false
	}
	_, err := hex.DecodeString(storedHash)
	return err == nil
}

func IsArgon2idHash(storedHash string) bool {
	return strings.HasPrefix(storedHash, argon2Prefix)
}

func encodePHC(salt, hash []byte) string {
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version,
		argon2MemoryKiB,
		argon2Iterations,
		argon2Parallel,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

func verifyArgon2id(password, storedHash string) (bool, error) {
	salt, params, expected, err := parsePHC(storedHash)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(password), salt, params.iterations, params.memory, params.parallelism, uint32(len(expected)))
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return false, nil
	}
	return true, nil
}

type phcParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func parsePHC(storedHash string) (salt []byte, params phcParams, hash []byte, err error) {
	if !strings.HasPrefix(storedHash, argon2Prefix) {
		return nil, phcParams{}, nil, errors.New("invalid argon2id prefix")
	}
	rest := strings.TrimPrefix(storedHash, argon2Prefix)
	parts := strings.Split(rest, "$")
	if len(parts) != 4 {
		return nil, phcParams{}, nil, errors.New("invalid phc segments")
	}
	if parts[0] != fmt.Sprintf("v=%d", argon2Version) {
		return nil, phcParams{}, nil, errors.New("unsupported argon2 version")
	}
	params, err = parseParams(parts[1])
	if err != nil {
		return nil, phcParams{}, nil, err
	}
	if err = validateArgon2Params(params); err != nil {
		return nil, phcParams{}, nil, err
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) == 0 || len(salt) > maxArgon2SaltLen {
		return nil, phcParams{}, nil, errors.New("invalid salt")
	}
	hash, err = base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(hash) == 0 || len(hash) > maxArgon2HashLen {
		return nil, phcParams{}, nil, errors.New("invalid hash")
	}
	return salt, params, hash, nil
}

func parseParams(segment string) (phcParams, error) {
	var p phcParams
	for _, kv := range strings.Split(segment, ",") {
		pair := strings.SplitN(kv, "=", 2)
		if len(pair) != 2 {
			return phcParams{}, errors.New("invalid param")
		}
		switch pair[0] {
		case "m":
			v, err := strconv.ParseUint(pair[1], 10, 32)
			if err != nil {
				return phcParams{}, err
			}
			p.memory = uint32(v)
		case "t":
			v, err := strconv.ParseUint(pair[1], 10, 32)
			if err != nil {
				return phcParams{}, err
			}
			p.iterations = uint32(v)
		case "p":
			v, err := strconv.ParseUint(pair[1], 10, 8)
			if err != nil {
				return phcParams{}, err
			}
			p.parallelism = uint8(v)
		default:
			return phcParams{}, errors.New("unknown param")
		}
	}
	if p.memory == 0 || p.iterations == 0 || p.parallelism == 0 {
		return phcParams{}, errors.New("missing param")
	}
	return p, nil
}

func validateArgon2Params(p phcParams) error {
	if p.memory > maxArgon2MemoryKiB {
		return errors.New("argon2 memory exceeds limit")
	}
	if p.iterations > maxArgon2Iterations {
		return errors.New("argon2 iterations exceeds limit")
	}
	if p.parallelism > maxArgon2Parallel {
		return errors.New("argon2 parallelism exceeds limit")
	}
	return nil
}

func legacySHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}