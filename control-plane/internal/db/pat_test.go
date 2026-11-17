package db

import (
	"strings"
	"testing"
)

func TestNewPersonalAccessToken(t *testing.T) {
	plain, hash, prefix, err := NewPersonalAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, "norix_pat_") {
		t.Fatalf("prefix: %s", plain)
	}
	if len(hash) != 32 {
		t.Fatalf("hash len %d", len(hash))
	}
	if !strings.HasPrefix(plain, prefix) {
		t.Fatalf("token_prefix %q not prefix of %q", prefix, plain)
	}
}
