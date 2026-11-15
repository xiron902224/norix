package scheduler

import (
	"fmt"
	"sort"
	"strings"
)

// NormalizeNodeRole returns worker|edge|control|… trimmed lowercase.
// Empty role defaults to worker (S7 node roles).
func NormalizeNodeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return "worker"
	}
	return role
}

// IsSchedulableRole reports whether a node role may receive workloads.
func IsSchedulableRole(role string) bool {
	switch NormalizeNodeRole(role) {
	case "worker", "edge":
		return true
	default:
		return false
	}
}

// Node is a scheduling candidate snapshot.
type Node struct {
	ID               string
	Name             string
	Status           string
	Role             string
	Cordoned         bool
	Draining         bool
	Zone             string
	Labels           map[string]string
	IsLocal          bool
	CPUCores         int32
	MemoryTotalMB    int64
	MemoryFreeMB     int64
	DockerSocket     bool
	ContainerdSocket bool
	KVMLevel         uint32
	CPUUsagePercent  float64
	AssignedCount    int
	AssignedMemoryMB int64
}

// Request describes placement constraints for one replica.
type Request struct {
	WorkloadID          string
	MemoryLimitMB       int64
	RequireKVM          bool
	Strategy            string // spread | binpack
	PreferredZone       string
	AffinityLabels      map[string]string
	AntiAffinityLabels  map[string]string
	ExcludeNodeIDs      map[string]struct{}
	AvoidNodesWithSame  map[string]struct{} // node IDs already holding sibling replicas (spread)
	AllowCordoned       bool
	AllowDraining       bool
	PreferRemote        bool
}

// Select picks the best feasible node or returns an error explaining unschedulable.
func Select(nodes []Node, req Request) (Node, error) {
	feasible := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		if ok, _ := Feasible(node, req); ok {
			feasible = append(feasible, node)
		}
	}
	if len(feasible) == 0 {
		return Node{}, fmt.Errorf("unschedulable: no feasible node for workload constraints")
	}
	sort.SliceStable(feasible, func(i, j int) bool {
		si := Score(feasible[i], req)
		sj := Score(feasible[j], req)
		if si == sj {
			return feasible[i].Name < feasible[j].Name
		}
		return si > sj
	})
	return feasible[0], nil
}

// Feasible applies hard filters.
func Feasible(node Node, req Request) (bool, string) {
	if req.ExcludeNodeIDs != nil {
		if _, skip := req.ExcludeNodeIDs[node.ID]; skip {
			return false, "excluded"
		}
	}
	if node.Status != "online" {
		return false, "node offline"
	}
	if !req.AllowCordoned && node.Cordoned {
		return false, "cordoned"
	}
	if !req.AllowDraining && node.Draining {
		return false, "draining"
	}
	role := NormalizeNodeRole(node.Role)
	if role != "worker" && role != "edge" {
		return false, "invalid role"
	}
	// Edge and worker are both schedulable for container workloads (S7).
	if !node.DockerSocket && !node.ContainerdSocket {
		return false, "no container runtime"
	}
	if req.RequireKVM && node.KVMLevel < 1 {
		return false, "kvm required"
	}
	free := node.MemoryFreeMB
	if free <= 0 && node.MemoryTotalMB > 0 {
		free = node.MemoryTotalMB - node.AssignedMemoryMB
	}
	need := req.MemoryLimitMB
	if need <= 0 {
		need = 512
	}
	if free > 0 && free < need {
		return false, "insufficient memory"
	}
	for key, want := range req.AffinityLabels {
		if node.Labels[key] != want {
			return false, "affinity miss"
		}
	}
	for key, forbidden := range req.AntiAffinityLabels {
		if node.Labels[key] == forbidden {
			return false, "anti-affinity"
		}
	}
	return true, ""
}

// Score ranks a feasible node. Higher is better.
func Score(node Node, req Request) int64 {
	strategy := strings.ToLower(strings.TrimSpace(req.Strategy))
	if strategy == "" {
		strategy = "spread"
	}
	var score int64
	free := node.MemoryFreeMB
	if free <= 0 && node.MemoryTotalMB > 0 {
		free = node.MemoryTotalMB - node.AssignedMemoryMB
		if free < 0 {
			free = 0
		}
	}
	switch strategy {
	case "binpack":
		// Prefer fuller nodes (less free memory / more assigned).
		score += int64(node.AssignedCount) * 1000
		score += node.AssignedMemoryMB
		score -= free
	default: // spread
		score -= int64(node.AssignedCount) * 10_000
		score += free
		if req.AvoidNodesWithSame != nil {
			if _, used := req.AvoidNodesWithSame[node.ID]; used {
				score -= 100_000
			}
		}
	}
	if req.PreferredZone != "" && strings.EqualFold(node.Zone, req.PreferredZone) {
		score += 5_000
	}
	if req.PreferRemote && !node.IsLocal {
		score += 2_000
	}
	if node.IsLocal {
		score -= 500
	}
	// Prefer healthier CPU headroom.
	score += int64(100 - node.CPUUsagePercent)
	if node.KVMLevel > 0 {
		score += 50
	}
	return score
}
