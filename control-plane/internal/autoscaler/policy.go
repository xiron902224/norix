package autoscaler

import "fmt"

// Policy is S18 / #49 horizontal scaling rules.
type Policy struct {
	MinReplicas  int32
	MaxReplicas  int32
	CPUTarget    float64 // percent
	MemoryTarget float64 // percent of node memory used
	RPSTarget    float64
	CooldownMs    int64
}

func (p Policy) Validate() error {
	if p.MinReplicas < 1 {
		return fmt.Errorf("min_replicas must be >= 1")
	}
	if p.MaxReplicas < p.MinReplicas {
		return fmt.Errorf("max_replicas must be >= min_replicas")
	}
	if p.CPUTarget <= 0 && p.RPSTarget <= 0 && p.MemoryTarget <= 0 {
		return fmt.Errorf("cpu_target, memory_target, or rps_target required")
	}
	if p.CooldownMs < 0 {
		return fmt.Errorf("cooldown_ms must be >= 0")
	}
	return nil
}

// DesiredReplicas computes next replica count from observed signals.
// Scale-up when any configured signal exceeds its target; scale-down when CPU
// (if configured) is under half its target and no other signal is hot.
func DesiredReplicas(p Policy, current int32, cpuPercent, rps, memPercent float64) int32 {
	_ = p.Validate()
	desired := current
	hot := false
	if p.CPUTarget > 0 && cpuPercent > p.CPUTarget {
		desired++
		hot = true
	}
	if p.RPSTarget > 0 && rps > p.RPSTarget {
		desired++
		hot = true
	}
	if p.MemoryTarget > 0 && memPercent > p.MemoryTarget {
		desired++
		hot = true
	}
	if !hot && p.CPUTarget > 0 && cpuPercent < p.CPUTarget*0.5 && desired > p.MinReplicas {
		desired--
	}
	if !hot && p.CPUTarget <= 0 && p.MemoryTarget > 0 && memPercent < p.MemoryTarget*0.5 && desired > p.MinReplicas {
		desired--
	}
	if desired < p.MinReplicas {
		desired = p.MinReplicas
	}
	if desired > p.MaxReplicas {
		desired = p.MaxReplicas
	}
	return desired
}
