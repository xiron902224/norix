package autoscaler

import "testing"

func TestDesiredReplicasScaleUpDown(t *testing.T) {
	p := Policy{MinReplicas: 1, MaxReplicas: 5, CPUTarget: 70, CooldownMs: 1000}
	if DesiredReplicas(p, 1, 90, 0, 0) != 2 {
		t.Fatal("scale up")
	}
	if DesiredReplicas(p, 3, 20, 0, 0) != 2 {
		t.Fatal("scale down")
	}
	if DesiredReplicas(p, 5, 99, 0, 0) != 5 {
		t.Fatal("cap max")
	}
}

func TestDesiredReplicasMemory(t *testing.T) {
	p := Policy{MinReplicas: 1, MaxReplicas: 4, MemoryTarget: 80, CooldownMs: 1000}
	if DesiredReplicas(p, 1, 0, 0, 95) != 2 {
		t.Fatal("mem scale up")
	}
	if DesiredReplicas(p, 3, 0, 0, 20) != 2 {
		t.Fatal("mem scale down")
	}
}

func TestDesiredReplicasRPS(t *testing.T) {
	p := Policy{MinReplicas: 1, MaxReplicas: 4, RPSTarget: 100, CooldownMs: 0}
	if DesiredReplicas(p, 1, 0, 150, 0) != 2 {
		t.Fatal("rps scale up")
	}
}

func TestPolicyValidate(t *testing.T) {
	if (Policy{MinReplicas: 2, MaxReplicas: 1}).Validate() == nil {
		t.Fatal("expected error")
	}
	if (Policy{MinReplicas: 1, MaxReplicas: 2}).Validate() == nil {
		t.Fatal("need a target")
	}
	if err := (Policy{MinReplicas: 1, MaxReplicas: 2, MemoryTarget: 75}).Validate(); err != nil {
		t.Fatal(err)
	}
}
