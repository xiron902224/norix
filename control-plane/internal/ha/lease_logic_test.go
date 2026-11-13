package ha

import "testing"

func TestDefaultLeaseTTL(t *testing.T) {
	if DefaultLeaseTTL().Seconds() != 15 {
		t.Fatal("ttl")
	}
}
