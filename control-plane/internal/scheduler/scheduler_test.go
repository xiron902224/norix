package scheduler

import "testing"

func TestFeasibleRejectsNoKVMForMicroVM(t *testing.T) {
	node := Node{
		ID: "n1", Status: "online", Role: "worker",
		DockerSocket: true, KVMLevel: 0, MemoryFreeMB: 4096,
	}
	ok, reason := Feasible(node, Request{RequireKVM: true, MemoryLimitMB: 512})
	if ok {
		t.Fatalf("expected reject, got ok")
	}
	if reason != "kvm required" {
		t.Fatalf("reason=%s", reason)
	}
}

func TestSpreadPrefersEmptyNode(t *testing.T) {
	nodes := []Node{
		{ID: "a", Name: "a", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 2048, AssignedCount: 3},
		{ID: "b", Name: "b", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 2048, AssignedCount: 0},
	}
	picked, err := Select(nodes, Request{Strategy: "spread", MemoryLimitMB: 256, PreferRemote: true})
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "b" {
		t.Fatalf("want b, got %s", picked.ID)
	}
}

func TestBinpackPrefersFullerNode(t *testing.T) {
	nodes := []Node{
		{ID: "a", Name: "a", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 1024, AssignedCount: 0, AssignedMemoryMB: 0},
		{ID: "b", Name: "b", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 512, AssignedCount: 2, AssignedMemoryMB: 1024},
	}
	picked, err := Select(nodes, Request{Strategy: "binpack", MemoryLimitMB: 256})
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "b" {
		t.Fatalf("want b, got %s", picked.ID)
	}
}

func TestCordonedNotSchedulable(t *testing.T) {
	nodes := []Node{
		{ID: "a", Name: "a", Status: "online", Role: "worker", DockerSocket: true, Cordoned: true, MemoryFreeMB: 4096},
	}
	_, err := Select(nodes, Request{MemoryLimitMB: 256})
	if err == nil {
		t.Fatal("expected unschedulable")
	}
}

func TestAffinityAndAntiAffinity(t *testing.T) {
	nodes := []Node{
		{ID: "a", Name: "a", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 4096, Labels: map[string]string{"tier": "web"}},
		{ID: "b", Name: "b", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 4096, Labels: map[string]string{"tier": "db"}},
	}
	picked, err := Select(nodes, Request{
		MemoryLimitMB: 256,
		AffinityLabels: map[string]string{"tier": "db"},
		AntiAffinityLabels: map[string]string{"tier": "web"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "b" {
		t.Fatalf("want b, got %s", picked.ID)
	}
}
