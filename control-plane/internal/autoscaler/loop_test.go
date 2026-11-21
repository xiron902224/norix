package autoscaler

import (
	"testing"
	"time"
)

func TestCooldownTrackerRespectsCooldownMs(t *testing.T) {
	tr := NewCooldownTracker()
	now := time.Unix(1_700_000_000, 0)
	if !tr.Allow("w1", 30_000, now) {
		t.Fatal("first allow")
	}
	tr.Mark("w1", now)
	if tr.Allow("w1", 30_000, now.Add(10*time.Second)) {
		t.Fatal("should block inside cooldown")
	}
	if !tr.Allow("w1", 30_000, now.Add(31*time.Second)) {
		t.Fatal("should allow after cooldown")
	}
}

func TestDecideRespectsCooldown(t *testing.T) {
	p := Policy{MinReplicas: 1, MaxReplicas: 5, CPUTarget: 70, CooldownMs: 5_000}
	now := time.Unix(1_700_000_000, 0)
	desired, apply := Decide(p, 1, 90, 0, 0, now, now.Add(time.Second), true)
	if desired != 2 || apply {
		t.Fatalf("desired=%d apply=%v", desired, apply)
	}
	desired, apply = Decide(p, 1, 90, 0, 0, now, now.Add(6*time.Second), true)
	if desired != 2 || !apply {
		t.Fatalf("desired=%d apply=%v after cooldown", desired, apply)
	}
	desired, apply = Decide(p, 2, 40, 0, 0, time.Time{}, now, false)
	if apply {
		t.Fatalf("no change should not apply, desired=%d", desired)
	}
}

// TestFlappingBlockedByCooldown: high→scale up, then immediate low must not scale down.
func TestFlappingBlockedByCooldown(t *testing.T) {
	p := Policy{MinReplicas: 1, MaxReplicas: 8, CPUTarget: 70, CooldownMs: 30_000}
	now := time.Unix(1_700_000_000, 0)
	desired, apply := Decide(p, 1, 95, 0, 0, time.Time{}, now, false)
	if desired != 2 || !apply {
		t.Fatalf("scale up: desired=%d apply=%v", desired, apply)
	}
	// Immediately opposite signal (classic flap) — cooldown must hold.
	desired, apply = Decide(p, 2, 10, 0, 0, now, now.Add(2*time.Second), true)
	if desired != 1 {
		t.Fatalf("desired should be 1, got %d", desired)
	}
	if apply {
		t.Fatal("flap scale-down must be blocked by cooldown")
	}
	desired, apply = Decide(p, 2, 10, 0, 0, now, now.Add(31*time.Second), true)
	if desired != 1 || !apply {
		t.Fatalf("after cooldown desired=%d apply=%v", desired, apply)
	}
}

func TestPolicyForWorkloadOverrides(t *testing.T) {
	base := Policy{MinReplicas: 1, MaxReplicas: 8, CPUTarget: 70, CooldownMs: 30_000}
	p := PolicyForWorkload(base, map[string]string{
		"NORIX_AUTOSCALE_MIN":         "2",
		"NORIX_AUTOSCALE_MAX":         "4",
		"NORIX_AUTOSCALE_CPU_TARGET":  "80",
		"NORIX_AUTOSCALE_MEM_TARGET":  "85",
		"NORIX_AUTOSCALE_RPS_TARGET":  "200",
		"NORIX_AUTOSCALE_COOLDOWN_MS": "1000",
	})
	if p.MinReplicas != 2 || p.MaxReplicas != 4 || p.CPUTarget != 80 || p.MemoryTarget != 85 || p.RPSTarget != 200 || p.CooldownMs != 1000 {
		t.Fatalf("%+v", p)
	}
}

func TestWorkloadAutoscalerEnabled(t *testing.T) {
	if !WorkloadAutoscalerEnabled(nil, true) {
		t.Fatal("scale all")
	}
	if WorkloadAutoscalerEnabled(map[string]string{"NORIX_AUTOSCALE": "0"}, true) {
		t.Fatal("opt out")
	}
	if !WorkloadAutoscalerEnabled(map[string]string{"NORIX_AUTOSCALE": "1"}, false) {
		t.Fatal("opt in")
	}
	if WorkloadAutoscalerEnabled(map[string]string{}, false) {
		t.Fatal("no opt-in when scaleAll false")
	}
}
