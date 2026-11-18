package db

import "testing"

func TestComposeProjectNameIsStableAndShort(t *testing.T) {
	got := ComposeProjectName("3F2504E0-4F89-11D3-9A0C-0305E82C3301")
	if got != "norix-3f2504e04f89" {
		t.Fatalf("ComposeProjectName = %q", got)
	}
}

func TestComposeStackActionDesired(t *testing.T) {
	cases := map[string]string{"start": "up", "redeploy": "up", "stop": "stop", "delete": "down", " Stop ": "stop"}
	for action, want := range cases {
		got, err := ComposeStackActionDesired(action)
		if err != nil || got != want {
			t.Fatalf("ComposeStackActionDesired(%q) = %q, %v; want %q", action, got, err, want)
		}
	}
	if _, err := ComposeStackActionDesired("scale"); err == nil {
		t.Fatal("unknown action must fail")
	}
}

func TestResolveObservedStatus(t *testing.T) {
	cases := []struct {
		name       string
		action     string
		desiredGen int64
		observed   ObservedComposeStack
		want       string
	}{
		{"current running", "up", 2, ObservedComposeStack{Status: "running", ObservedGeneration: 2}, "running"},
		{"stale running hides nothing", "up", 3, ObservedComposeStack{Status: "running", ObservedGeneration: 2}, "pending"},
		{"stale report while deleting", "down", 3, ObservedComposeStack{Status: "running", ObservedGeneration: 2}, "removing"},
		{"deploying is always shown", "up", 3, ObservedComposeStack{Status: "deploying", ObservedGeneration: 2}, "deploying"},
		{"unknown status becomes failed", "up", 1, ObservedComposeStack{Status: "weird", ObservedGeneration: 1}, "failed"},
	}
	for _, tc := range cases {
		if got := ResolveObservedStatus(tc.action, tc.desiredGen, tc.observed); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}
