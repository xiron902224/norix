package db

import "testing"

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
