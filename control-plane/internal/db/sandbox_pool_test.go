package db

import "testing"

func TestWarmPoolTarget(t *testing.T) {
	t.Setenv("NORIX_SANDBOX_WARM_POOL", "")
	t.Setenv("NORIX_SANDBOX_WARM_POOL_SIZE", "")
	if got := WarmPoolTarget(); got != 2 {
		t.Fatalf("default = %d", got)
	}
	t.Setenv("NORIX_SANDBOX_WARM_POOL_SIZE", "5")
	if got := WarmPoolTarget(); got != 5 {
		t.Fatalf("size env = %d", got)
	}
	t.Setenv("NORIX_SANDBOX_WARM_POOL_SIZE", "")
	t.Setenv("NORIX_SANDBOX_WARM_POOL", "3")
	if got := WarmPoolTarget(); got != 3 {
		t.Fatalf("pool env = %d", got)
	}
}

func TestSandboxContainerName(t *testing.T) {
	name := SandboxContainerName("11111111-2222-3333-4444-555555555555")
	if name != "norix-sandbox-111111112222" {
		t.Fatalf("name = %q", name)
	}
}
