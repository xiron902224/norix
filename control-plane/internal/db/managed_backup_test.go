package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedBackupDirUsesNORIX_BACKUP_DIR(t *testing.T) {
	t.Setenv("NORIX_BACKUP_DIR", "/tmp/norix-test-backups")
	item := ManagedDatabase{ID: "db-abc", VolumePath: "/ignored"}
	got := managedBackupDir(item)
	want := filepath.Join("/tmp/norix-test-backups", "managed", "db-abc")
	if got != want {
		t.Fatalf("managedBackupDir = %q, want %q", got, want)
	}
}

func TestWriteManagedDumpLocalFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NORIX_BACKUP_DIR", dir)
	t.Setenv("NORIX_MANAGED_PG_DSN", "")
	t.Setenv("NORIX_BACKUP_S3_BUCKET", "")

	item := ManagedDatabase{
		ID:         "11111111-1111-1111-1111-111111111111",
		Engine:     "postgres",
		Name:       "appdb",
		VolumePath: filepath.Join(dir, "vol"),
	}
	path, err := writeManagedDump(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, filepath.Join(dir, "managed", item.ID)) {
		t.Fatalf("dump path %q not under backup dir", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "engine=postgres") && !strings.Contains(string(data), "PostgreSQL") {
		snippet := string(data)
		if len(snippet) > 80 {
			snippet = snippet[:80]
		}
		t.Fatalf("expected postgres dump marker, got %q", snippet)
	}
	newest, err := newestManagedDump(item)
	if err != nil {
		t.Fatal(err)
	}
	if newest != path {
		t.Fatalf("newest = %q, want %q", newest, path)
	}
}

func TestWriteManagedDumpRedisLocal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NORIX_BACKUP_DIR", dir)
	item := ManagedDatabase{ID: "redis-1", Engine: "redis", Name: "cache"}
	path, err := writeManagedDump(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "NORIX-REDIS-BACKUP") {
		t.Fatalf("unexpected redis dump: %s", data)
	}
}

func TestBackupS3PrefixDefault(t *testing.T) {
	t.Setenv("NORIX_BACKUP_S3_PREFIX", "")
	t.Setenv("NORIX_BACKUP_PREFIX", "")
	if got := backupS3Prefix(); got != "norix/managed" {
		t.Fatalf("prefix = %q", got)
	}
	t.Setenv("NORIX_BACKUP_S3_PREFIX", "backups/prod")
	if got := backupS3Prefix(); got != "backups/prod" {
		t.Fatalf("prefix = %q", got)
	}
}

func TestApplyManagedDumpWithoutDSN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.dump")
	if err := os.WriteFile(path, []byte("-- dump\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	item := ManagedDatabase{Engine: "postgres", Name: "x"}
	if err := applyManagedDump(context.Background(), item, path); err != nil {
		t.Fatal(err)
	}
}
