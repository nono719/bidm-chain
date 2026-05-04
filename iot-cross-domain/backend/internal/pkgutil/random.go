package pkgutil

import (
	"crypto/rand"
	"encoding/hex"
)

func RandomHex(length int) string {
	if length <= 0 {
		length = 16
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "fallback-random-value"
	}
	return hex.EncodeToString(buf)
}
