package db

import "testing"

func TestNormalizeUpdateStrategy(t *testing.T) {
	s, surge, unavail := NormalizeUpdateStrategy("", 0, 0)
	if s != "rolling" || surge != 1 || unavail != 0 {
		t.Fatalf("defaults: got %s surge=%d unavail=%d", s, surge, unavail)
	}
	s, surge, unavail = NormalizeUpdateStrategy("recreate", 0, 0)
	if s != "recreate" || surge != 0 || unavail != 0 {
		t.Fatalf("recreate: got %s surge=%d unavail=%d", s, surge, unavail)
	}
	s, surge, unavail = NormalizeUpdateStrategy("ROLLING", 2, 1)
	if s != "rolling" || surge != 2 || unavail != 1 {
		t.Fatalf("custom: got %s surge=%d unavail=%d", s, surge, unavail)
	}
}
