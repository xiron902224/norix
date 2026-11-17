package secrets

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 32)
	plain := []byte("wg-private-key-material")
	ct, nonce, err := Encrypt(key, plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(nonce) != 12 {
		t.Fatalf("nonce length %d, want 12", len(nonce))
	}
	got, err := Decrypt(key, ct, nonce)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("plaintext mismatch: got %q want %q", got, plain)
	}
}

func TestDecryptWrongNonceLengthReturnsError(t *testing.T) {
	key := bytes.Repeat([]byte{0x22}, 32)
	ct, _, err := Encrypt(key, []byte("x"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Mimic legacy "dev-nonce" (9 bytes) — must not panic.
	_, err = Decrypt(key, ct, []byte(DevNonceMarker))
	if err == nil {
		t.Fatal("expected error for wrong nonce length")
	}
}

func TestIsDevNonce(t *testing.T) {
	if !IsDevNonce([]byte(DevNonceMarker)) {
		t.Fatal("expected DevNonceMarker to match")
	}
	if IsDevNonce([]byte("not-dev")) {
		t.Fatal("unexpected match")
	}
}
