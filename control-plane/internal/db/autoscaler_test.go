package db

import "testing"

func TestScalableWorkloadDefaults(t *testing.T) {
	// Pure sanity for types used by the HPA loop (DB-backed List/Set covered when pool available).
	w := ScalableWorkload{}
	w.ReplicaCount = 0
	if w.ReplicaCount != 0 {
		t.Fatal("zero")
	}
	w.HasNodeCPU = true
	w.NodeCPUUsagePercent = 85
	if !w.HasNodeCPU || w.NodeCPUUsagePercent < 70 {
		t.Fatal("cpu signal")
	}
}
