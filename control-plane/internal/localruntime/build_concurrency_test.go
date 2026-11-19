package localruntime

import "testing"

func TestBuildConcurrencyDefaultsAndClamps(t *testing.T) {
	cases := map[string]int{
		"":    defaultBuildConcurrency,
		"abc": defaultBuildConcurrency,
		"0":   defaultBuildConcurrency,
		"-3":  defaultBuildConcurrency,
		"4":   4,
		" 3 ": 3,
		"99":  maxBuildConcurrency,
	}
	for raw, want := range cases {
		t.Setenv("NORIX_BUILD_CONCURRENCY", raw)
		if got := buildConcurrency(); got != want {
			t.Fatalf("NORIX_BUILD_CONCURRENCY=%q: got %d want %d", raw, got, want)
		}
	}
}
