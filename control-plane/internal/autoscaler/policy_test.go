package autoscaler

import "testing"

func TestDesiredReplicasScaleUpDown(t *testing.T) {
	p := Policy{MinReplicas: 1, MaxReplicas: 5, CPUTarget: 70, CooldownMs: 1000}
	if DesiredReplicas(p, 1, 90, 0) != 2 {
		t.Fatal("scale up")
	}
	if DesiredReplicas(p, 3, 20, 0) != 2 {
		t.Fatal("scale down")
	}
	if DesiredReplicas(p, 5, 99, 0) != 5 {
		t.Fatal("cap max")
	}
}

func TestPolicyValidate(t *testing.T) {
	if (Policy{MinReplicas: 2, MaxReplicas: 1}).Validate() == nil {
		t.Fatal("expected error")
	}
}
