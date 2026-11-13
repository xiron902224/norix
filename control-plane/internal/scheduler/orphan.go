package scheduler

import "strings"

// OrphanReplica is a minimal view used to decide S7 reschedule eligibility
// without hitting the database (mirrors RescheduleOrphanReplicas filters).
type OrphanReplica struct {
	AssignedNodeID string
	DesiredAction  string
	Status         string
	NodeStatus     string // empty if node missing
	NodeDraining   bool
	NodeExists     bool
}

// ShouldReschedule reports whether a replica must leave its current node
// (offline / missing / draining) under S7 rules.
func ShouldReschedule(r OrphanReplica) bool {
	nodeDead := !r.NodeExists || r.AssignedNodeID == "" || strings.ToLower(strings.TrimSpace(r.NodeStatus)) != "online"
	draining := r.NodeExists && r.NodeDraining

	if !nodeDead && !draining {
		return false
	}

	action := strings.ToLower(strings.TrimSpace(r.DesiredAction))
	status := strings.ToLower(strings.TrimSpace(r.Status))

	if action == "run" || action == "stop" {
		return true
	}
	settled := status == "running" || status == "deploying" || status == "stopped" || status == "failed" || status == "pending"
	if action == "none" && settled && nodeDead {
		return true
	}
	if draining && settled {
		return true
	}
	return false
}
