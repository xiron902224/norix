package db

import (
	"os"
	"testing"
)

func TestNetworkApplyEnabledDefaultOn(t *testing.T) {
	t.Setenv("NORIX_APPLY_WIREGUARD", "")
	if !NetworkApplyEnabled() {
		t.Fatal("empty env should default to apply on (#48)")
	}
	t.Setenv("NORIX_APPLY_WIREGUARD", "1")
	if !NetworkApplyEnabled() {
		t.Fatal("1")
	}
	for _, off := range []string{"0", "false", "off", "no", "dry-run"} {
		t.Setenv("NORIX_APPLY_WIREGUARD", off)
		if NetworkApplyEnabled() {
			t.Fatalf("%s should disable apply", off)
		}
	}
	_ = os.Unsetenv("NORIX_APPLY_WIREGUARD")
}
