package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// KeyFromEnv loads a 32-byte AES key from NORIX_BUILD_SECRETS_KEY (64 hex chars).
// Empty key disables encryption and returns an error on Encrypt.
func KeyFromEnv() ([]byte, error) {
	raw := os.Getenv("NORIX_BUILD_SECRETS_KEY")
	if raw == "" {
		return nil, fmt.Errorf("NORIX_BUILD_SECRETS_KEY is not set")
	}
	key, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode NORIX_BUILD_SECRETS_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("NORIX_BUILD_SECRETS_KEY must be 32 bytes (64 hex chars)")
	}
	return key, nil
}

func Encrypt(key, plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, plaintext, nil), nonce, nil
}

func Decrypt(key, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}
