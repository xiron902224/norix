package limits

import (
	"strings"
	"testing"
)

func TestDefaultCaps(t *testing.T) {
	c := Default()
	if c.MaxReplicas != 32 || c.MaxAutoscalerReplicas != 8 {
		t.Fatalf("defaults %+v", c)
	}
	if err := c.CheckReplicas(32); err != nil {
		t.Fatal(err)
	}
	if err := c.CheckReplicas(33); err == nil || !strings.Contains(err.Error(), "NORIX_MAX_REPLICAS") {
		t.Fatalf("err=%v", err)
	}
	if err := c.CheckReplicas(0); err == nil {
		t.Fatal("zero replicas must fail")
	}
}

func TestFromEnvAndClamp(t *testing.T) {
	t.Setenv("NORIX_MAX_REPLICAS", "16")
	t.Setenv("NORIX_MAX_AUTOSCALER_REPLICAS", "20")
	t.Setenv("NORIX_MAX_NODES_PER_ORG", "4")
	c := FromEnv()
	if c.MaxReplicas != 16 {
		t.Fatalf("replicas %d", c.MaxReplicas)
	}
	// Autoscaler cannot exceed MaxReplicas.
	if c.MaxAutoscalerReplicas != 16 {
		t.Fatalf("autoscaler clamp %d", c.MaxAutoscalerReplicas)
	}
	if c.MaxNodesPerOrg != 4 {
		t.Fatalf("nodes %d", c.MaxNodesPerOrg)
	}
	if got := c.ClampAutoscalerMax(99); got != 16 {
		t.Fatalf("clamp %d", got)
	}
}

func TestBadEnvFallsBack(t *testing.T) {
	t.Setenv("NORIX_MAX_REPLICAS", "nope")
	c := FromEnv()
	if c.MaxReplicas != 32 {
		t.Fatalf("fallback %d", c.MaxReplicas)
	}
}
