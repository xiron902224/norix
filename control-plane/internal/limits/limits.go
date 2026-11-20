// Package limits holds configurable hard caps for nodes, projects, workloads,
// and replicas (#45). Defaults match what the product already documented
// informally (replica_count ≤ 32, autoscaler max 8).
package limits

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Caps are org-wide (nodes/projects) or per-project (workloads) hard limits.
type Caps struct {
	MaxNodesPerOrg         int
	MaxProjectsPerOrg      int
	MaxWorkloadsPerProject int
	MaxReplicas            int
	MaxAutoscalerReplicas  int
}

// Default returns the documented beta.6 supported scale.
func Default() Caps {
	return Caps{
		MaxNodesPerOrg:         32,
		MaxProjectsPerOrg:      100,
		MaxWorkloadsPerProject: 100,
		MaxReplicas:            32,
		MaxAutoscalerReplicas:  8,
	}
}

// FromEnv overlays NORIX_MAX_* variables on Default.
func FromEnv() Caps {
	c := Default()
	c.MaxNodesPerOrg = envInt("NORIX_MAX_NODES_PER_ORG", c.MaxNodesPerOrg)
	c.MaxProjectsPerOrg = envInt("NORIX_MAX_PROJECTS_PER_ORG", c.MaxProjectsPerOrg)
	c.MaxWorkloadsPerProject = envInt("NORIX_MAX_WORKLOADS_PER_PROJECT", c.MaxWorkloadsPerProject)
	c.MaxReplicas = envInt("NORIX_MAX_REPLICAS", c.MaxReplicas)
	c.MaxAutoscalerReplicas = envInt("NORIX_MAX_AUTOSCALER_REPLICAS", c.MaxAutoscalerReplicas)
	if c.MaxAutoscalerReplicas > c.MaxReplicas {
		c.MaxAutoscalerReplicas = c.MaxReplicas
	}
	return c
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return fallback
	}
	return v
}

// CheckReplicas returns an error when replicaCount is outside 1..MaxReplicas.
func (c Caps) CheckReplicas(replicaCount int32) error {
	if replicaCount < 1 {
		return fmt.Errorf("replica_count must be at least 1")
	}
	if int(replicaCount) > c.MaxReplicas {
		return fmt.Errorf("replica_count %d exceeds NORIX_MAX_REPLICAS=%d", replicaCount, c.MaxReplicas)
	}
	return nil
}

// ClampAutoscalerMax returns min(requested, MaxAutoscalerReplicas, MaxReplicas).
func (c Caps) ClampAutoscalerMax(requested int32) int32 {
	max := c.MaxAutoscalerReplicas
	if c.MaxReplicas < max {
		max = c.MaxReplicas
	}
	if requested < 1 {
		return int32(max)
	}
	if int(requested) > max {
		return int32(max)
	}
	return requested
}
