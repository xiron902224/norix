package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const SessionTokenBytes = 32

func NewSessionToken() (plainText string, hash []byte, err error) {
	bytes := make([]byte, SessionTokenBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}

	plainText = base64.RawURLEncoding.EncodeToString(bytes)
	digest := sha256.Sum256([]byte(plainText))
	return plainText, digest[:], nil
}

func HashSessionToken(token string) []byte {
	digest := sha256.Sum256([]byte(token))
	return digest[:]
}
