package db

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeDecodeEnvironmentRoundTrip(t *testing.T) {
	t.Setenv("NORIX_BUILD_SECRETS_KEY", strings.Repeat("ab", 32))
	t.Setenv("NORIX_ALLOW_INSECURE_DEV_SECRETS", "")

	plain := map[string]string{"DATABASE_URL": "postgres://secret", "EMPTY": ""}
	raw, err := encodeEnvironmentJSON(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !isEncryptedEnvJSON(string(raw)) {
		t.Fatalf("expected encrypted envelope, got %s", raw)
	}
	if strings.Contains(string(raw), "postgres://secret") {
		t.Fatal("plaintext leaked into stored JSON")
	}
	got, err := decodeEnvironmentJSON(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got["DATABASE_URL"] != "postgres://secret" {
		t.Fatalf("got %#v", got)
	}
}

func TestDecodeLegacyPlaintext(t *testing.T) {
	t.Setenv("NORIX_BUILD_SECRETS_KEY", strings.Repeat("cd", 32))
	got, err := decodeEnvironmentJSON(`{"FOO":"bar"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got["FOO"] != "bar" {
		t.Fatalf("got %#v", got)
	}
}

func TestMaskAndMerge(t *testing.T) {
	masked := MaskEnvironment(map[string]string{"A": "secret", "B": "x"})
	if masked["A"] != EnvVarMaskedPlaceholder || masked["B"] != EnvVarMaskedPlaceholder {
		t.Fatalf("mask %#v", masked)
	}
	merged := MergeEnvironmentUpdate(
		map[string]string{"A": "secret", "B": "old"},
		map[string]string{"A": EnvVarMaskedPlaceholder, "B": "new", "C": "fresh"},
	)
	if merged["A"] != "secret" || merged["B"] != "new" || merged["C"] != "fresh" {
		t.Fatalf("merge %#v", merged)
	}
}

func TestEncodeRequiresKey(t *testing.T) {
	t.Setenv("NORIX_BUILD_SECRETS_KEY", "")
	t.Setenv("NORIX_ALLOW_INSECURE_DEV_SECRETS", "")
	_, err := encodeEnvironmentJSON(map[string]string{"A": "1"})
	if err == nil {
		t.Fatal("expected error without key")
	}
}

func TestEncodeInsecureDevPlaintext(t *testing.T) {
	t.Setenv("NORIX_BUILD_SECRETS_KEY", "")
	t.Setenv("NORIX_ALLOW_INSECURE_DEV_SECRETS", "1")
	raw, err := encodeEnvironmentJSON(map[string]string{"A": "1"})
	if err != nil {
		t.Fatal(err)
	}
	var plain map[string]string
	if err := json.Unmarshal(raw, &plain); err != nil {
		t.Fatal(err)
	}
	if plain["A"] != "1" {
		t.Fatalf("got %#v", plain)
	}
}
