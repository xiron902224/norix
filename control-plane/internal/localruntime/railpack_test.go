package localruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRailpackSource(t *testing.T) {
	dir := t.TempDir()
	if _, _, ok := detectRailpackSource(dir); ok {
		t.Fatal("empty dir should not match")
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	kind, reason, ok := detectRailpackSource(dir)
	if !ok || kind != "railpack" || reason == "" {
		t.Fatalf("got kind=%q reason=%q ok=%v", kind, reason, ok)
	}

	goDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	kind, _, ok = detectRailpackSource(goDir)
	if !ok || kind != "railpack" {
		t.Fatalf("go.mod: kind=%q ok=%v", kind, ok)
	}

	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	kind, reason, ok = detectRailpackSource(staticDir)
	if !ok || kind != "railpack" || reason == "" {
		t.Fatalf("static: kind=%q reason=%q ok=%v", kind, reason, ok)
	}
}

func TestResolveBuildSourceFallsBackToRailpack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask==3.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	kind, path, logs := resolveBuildSource(dir, "Dockerfile")
	if kind != "railpack" {
		t.Fatalf("kind=%q path=%q logs=%v", kind, path, logs)
	}
}
