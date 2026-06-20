package auth

import (
	"crypto/rand"
	"encoding/base64"
	"io"
)

func RandomState(r io.Reader) (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func NewRandomState() (string, error) {
	return RandomState(rand.Reader)
}
