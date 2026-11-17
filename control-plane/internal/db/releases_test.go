package db

import "testing"

func TestExtractImageDigest(t *testing.T) {
	if got := extractImageDigest("nginx:alpine"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	want := "sha256:abc"
	if got := extractImageDigest("ghcr.io/org/app@" + want); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestReleaseRetentionDefault(t *testing.T) {
	t.Setenv("NORIX_RELEASE_RETENTION", "")
	if ReleaseRetention() != 10 {
		t.Fatalf("default retention want 10")
	}
	t.Setenv("NORIX_RELEASE_RETENTION", "3")
	if ReleaseRetention() != 3 {
		t.Fatalf("want 3")
	}
}

func TestAutoRollbackEnabled(t *testing.T) {
	t.Setenv("NORIX_AUTO_ROLLBACK", "")
	if AutoRollbackEnabled() {
		t.Fatal("expected false")
	}
	t.Setenv("NORIX_AUTO_ROLLBACK", "1")
	if !AutoRollbackEnabled() {
		t.Fatal("expected true")
	}
}
