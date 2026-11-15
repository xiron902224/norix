package api

import "testing"

func TestSanitizePromName(t *testing.T) {
	cases := map[string]string{
		"node.cpu-usage": "node_cpu_usage",
		"":               "norix_metric",
		"9bad":           "norix_metric",
		"ok_name":        "ok_name",
		"a b":            "a_b",
	}
	for in, want := range cases {
		if got := sanitizePromName(in); got != want {
			t.Fatalf("%q → %q, want %q", in, got, want)
		}
	}
}
