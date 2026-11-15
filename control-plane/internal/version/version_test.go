package version

import (
	"strings"
	"testing"
)

func TestCurrentPrefersEmbeddedWhenUnset(t *testing.T) {
	t.Setenv("NORIX_VERSION", "")
	BuildVersion = ""
	want := strings.TrimSpace(embedded)
	if got := Current(); got != want {
		t.Fatalf("Current()=%q want %q", got, want)
	}
}

func TestCurrentPrefersEnv(t *testing.T) {
	t.Setenv("NORIX_VERSION", "v9.9.9-test")
	BuildVersion = "0.0.1"
	if got := Current(); got != "9.9.9-test" {
		t.Fatalf("got %q", got)
	}
}
