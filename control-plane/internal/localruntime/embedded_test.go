package localruntime

import "testing"

func TestEmbeddedRuntimeEnabled(t *testing.T) {
	t.Setenv(EmbeddedRuntimeEnv, "")
	if EmbeddedRuntimeEnabled() {
		t.Fatal("expected disabled by default")
	}
	for _, v := range []string{"1", "true", "YES", "on"} {
		t.Setenv(EmbeddedRuntimeEnv, v)
		if !EmbeddedRuntimeEnabled() {
			t.Fatalf("expected enabled for %q", v)
		}
	}
	t.Setenv(EmbeddedRuntimeEnv, "0")
	if EmbeddedRuntimeEnabled() {
		t.Fatal("expected disabled for 0")
	}
}
