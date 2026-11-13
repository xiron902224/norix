package chaos

import "testing"

func TestChaosCatalog(t *testing.T) {
	if len(All()) < 4 {
		t.Fatal("expected baseline scenarios")
	}
}
