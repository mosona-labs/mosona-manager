package utils

import (
	"math/rand"
)

func RandomString(n int) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func RandomNumber[T int | int64](min, max T) T {
	switch any(min).(type) {
	case int:
		return T(int(min) + rand.Intn(int(max-min)))
	case int64:
		return T(int64(min) + rand.Int63n(int64(max-min)))
	default:
		panic("unsupported type")
	}
}
