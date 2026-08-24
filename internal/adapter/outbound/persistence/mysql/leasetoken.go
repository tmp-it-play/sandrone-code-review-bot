package mysql

import (
	"crypto/rand"
	"encoding/hex"
)

func newLeaseToken() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
