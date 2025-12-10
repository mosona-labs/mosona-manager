package utils

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
)

func SHA256(str string) string {
	hash := sha256.New()
	hash.Write([]byte(str))
	return hex.EncodeToString(hash.Sum(nil))
}

func MD5(str string) string {
	hash := md5.New()
	hash.Write([]byte(str))
	return hex.EncodeToString(hash.Sum(nil))
}
