package db

import (
	"strings"
	"testing"
)

func TestNormalizeManagedEngine(t *testing.T) {
	got, err := NormalizeManagedEngine("Postgres")
	if err != nil || got != "postgres" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := NormalizeManagedEngine("cassandra"); err == nil {
		t.Fatal("expected unsupported")
	}
}

func TestCatalogImageForEngine(t *testing.T) {
	img, err := CatalogImageForEngine("redis")
	if err != nil || img != "redis:7-alpine" {
		t.Fatalf("got %q err=%v", img, err)
	}
}

func TestDefaultBackupPath(t *testing.T) {
	path := DefaultBackupPath("/var/lib/norix/dbs/app", "abc")
	if path == "" || path[:len("/var/lib/norix/dbs/app/backups/")] != "/var/lib/norix/dbs/app/backups/" {
		t.Fatalf("unexpected path %s", path)
	}
}

func TestBuildManagedSecret(t *testing.T) {
	secret, redacted, err := buildManagedSecret("postgres", "app-db", "secret", 5433)
	if err != nil {
		t.Fatal(err)
	}
	if secret.URI == "" || !strings.Contains(secret.URI, "5433") {
		t.Fatalf("uri=%q", secret.URI)
	}
	if !strings.Contains(redacted, "******") {
		t.Fatalf("redacted=%q", redacted)
	}
	if _, _, err := buildManagedSecret("mongodb", "x", "y", 1); err == nil {
		t.Fatal("expected mongodb error")
	}
}

func TestManagedDBNames(t *testing.T) {
	if got := ManagedDBContainerName("abcdef12-3456-7890-abcd-ef1234567890"); got != "norix-mdb-abcdef123456" {
		t.Fatalf("container=%q", got)
	}
}
