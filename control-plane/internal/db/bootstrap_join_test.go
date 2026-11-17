package db

import (
	"testing"

	"github.com/xiron902224/norix/control-plane/internal/auth"
)

func TestBootstrapJoinTokenEnvName(t *testing.T) {
	if BootstrapJoinTokenEnv != "NORIX_BOOTSTRAP_JOIN_TOKEN" {
		t.Fatalf("unexpected env name %q", BootstrapJoinTokenEnv)
	}
	// Hash must match enroll path (SHA-256 of plaintext).
	h := auth.HashSessionToken("sample-join-token")
	if len(h) != 32 {
		t.Fatalf("expected 32-byte hash, got %d", len(h))
	}
}

func TestSeedBootstrapJoinTokenNoopWithoutEnv(t *testing.T) {
	t.Setenv(BootstrapJoinTokenEnv, "")
	// nil pool is fine — we return before DB use when env empty.
	n, err := SeedBootstrapJoinToken(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 seeded, got %d", n)
	}
}
