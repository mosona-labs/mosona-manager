package utils

import (
	"crypto/rand"
	"math/big"
	"strings"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandomString(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]rune, n)
	maxV := big.NewInt(int64(len(letterRunes)))
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, maxV)
		if err != nil {
			return ""
		}
		b[i] = letterRunes[idx.Int64()]
	}
	return string(b)
}

func RandomNumber[T int | int64](min, max T) T {
	switch any(min).(type) {
	case int:
		mi := int64(any(min).(int))
		ma := int64(any(max).(int))
		if ma <= mi {
			return min
		}
		diff := big.NewInt(ma - mi)
		n, err := rand.Int(rand.Reader, diff)
		if err != nil {
			return min
		}
		return T(int(mi + n.Int64()))
	case int64:
		mi := any(min).(int64)
		ma := any(max).(int64)
		if ma <= mi {
			return min
		}
		diff := big.NewInt(ma - mi)
		n, err := rand.Int(rand.Reader, diff)
		if err != nil {
			return min
		}
		return T(mi + n.Int64())
	default:
		panic("unsupported type")
	}
}

func EscapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
