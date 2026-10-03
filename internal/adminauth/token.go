package adminauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

func NewSessionToken() (plain string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	return plain, sum[:], nil
}

func HashSessionToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
