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

func TestSelectNeverPlacesMicroVMOnNonKVM(t *testing.T) {
	nodes := []Node{
		{ID: "plain", Name: "plain", Status: "online", Role: "worker", DockerSocket: true, KVMLevel: 0, MemoryFreeMB: 8192},
		{ID: "kvm", Name: "kvm", Status: "online", Role: "worker", DockerSocket: true, KVMLevel: 2, MemoryFreeMB: 2048},
	}
	picked, err := Select(nodes, Request{RequireKVM: true, MemoryLimitMB: 512, Strategy: "spread"})
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "kvm" {
		t.Fatalf("want kvm node, got %s", picked.ID)
	}
	_, err = Select(nodes[:1], Request{RequireKVM: true, MemoryLimitMB: 512})
	if err == nil {
		t.Fatal("expected unschedulable when only non-KVM nodes exist")
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

func TestSpreadAvoidsSiblingReplicas(t *testing.T) {
	nodes := []Node{
		{ID: "a", Name: "a", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 4096, AssignedCount: 0},
		{ID: "b", Name: "b", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 4096, AssignedCount: 0},
	}
	picked, err := Select(nodes, Request{
		Strategy: "spread", MemoryLimitMB: 256,
		AvoidNodesWithSame: map[string]struct{}{"a": {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "b" {
		t.Fatalf("want b (avoid sibling on a), got %s", picked.ID)
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

func TestDrainingNotSchedulable(t *testing.T) {
	nodes := []Node{
		{ID: "a", Name: "a", Status: "online", Role: "worker", DockerSocket: true, Draining: true, MemoryFreeMB: 4096},
		{ID: "b", Name: "b", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 2048},
	}
	picked, err := Select(nodes, Request{MemoryLimitMB: 256})
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "b" {
		t.Fatalf("want b, got %s", picked.ID)
	}
}

func TestAffinityAndAntiAffinity(t *testing.T) {
	nodes := []Node{
		{ID: "a", Name: "a", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 4096, Labels: map[string]string{"tier": "web"}},
		{ID: "b", Name: "b", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 4096, Labels: map[string]string{"tier": "db"}},
	}
	picked, err := Select(nodes, Request{
		MemoryLimitMB:      256,
		AffinityLabels:     map[string]string{"tier": "db"},
		AntiAffinityLabels: map[string]string{"tier": "web"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "b" {
		t.Fatalf("want b, got %s", picked.ID)
	}
}

func TestPreferredZoneBoost(t *testing.T) {
	nodes := []Node{
		{ID: "a", Name: "a", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 4096, Zone: "eu"},
		{ID: "b", Name: "b", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 4096, Zone: "us"},
	}
	picked, err := Select(nodes, Request{MemoryLimitMB: 256, PreferredZone: "us", Strategy: "spread"})
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "b" {
		t.Fatalf("want b (us zone), got %s", picked.ID)
	}
}

func TestWorkerAndEdgeRolesSchedulable(t *testing.T) {
	for _, role := range []string{"", "worker", "edge"} {
		ok, _ := Feasible(Node{
			ID: "n", Status: "online", Role: role, DockerSocket: true, MemoryFreeMB: 2048,
		}, Request{MemoryLimitMB: 256})
		if !ok {
			t.Fatalf("role %q should be schedulable", role)
		}
	}
	ok, reason := Feasible(Node{
		ID: "n", Status: "online", Role: "control", DockerSocket: true, MemoryFreeMB: 2048,
	}, Request{MemoryLimitMB: 256})
	if ok || reason != "invalid role" {
		t.Fatalf("control role should be rejected, ok=%v reason=%s", ok, reason)
	}
}

func TestNormalizeNodeRole(t *testing.T) {
	if NormalizeNodeRole("") != "worker" {
		t.Fatal("empty → worker")
	}
	if !IsSchedulableRole("EDGE") || !IsSchedulableRole("worker") {
		t.Fatal("worker/edge schedulable")
	}
	if IsSchedulableRole("control") {
		t.Fatal("control not schedulable")
	}
}

func TestExcludeOfflineNodeOnReschedule(t *testing.T) {
	nodes := []Node{
		{ID: "dead", Name: "dead", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 4096},
		{ID: "live", Name: "live", Status: "online", Role: "worker", DockerSocket: true, MemoryFreeMB: 4096},
	}
	picked, err := Select(nodes, Request{
		MemoryLimitMB: 256,
		ExcludeNodeIDs: map[string]struct{}{"dead": {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "live" {
		t.Fatalf("want live, got %s", picked.ID)
	}
}

func TestShouldRescheduleOfflineRunningNone(t *testing.T) {
	// Regression: running+none on offline node must move (S7 <60s path).
	if !ShouldReschedule(OrphanReplica{
		AssignedNodeID: "n1", DesiredAction: "none", Status: "running",
		NodeStatus: "offline", NodeExists: true,
	}) {
		t.Fatal("expected reschedule for offline running replica")
	}
	if ShouldReschedule(OrphanReplica{
		AssignedNodeID: "n1", DesiredAction: "none", Status: "running",
		NodeStatus: "online", NodeExists: true, NodeDraining: false,
	}) {
		t.Fatal("healthy running replica must stay")
	}
	if !ShouldReschedule(OrphanReplica{
		AssignedNodeID: "n1", DesiredAction: "none", Status: "running",
		NodeStatus: "online", NodeExists: true, NodeDraining: true,
	}) {
		t.Fatal("draining running replica must evacuate")
	}
}
