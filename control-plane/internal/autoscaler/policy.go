package autoscaler

import "fmt"

// Policy is S18 horizontal scaling rules.
type Policy struct {
	MinReplicas int32
	MaxReplicas int32
	CPUTarget   float64 // percent
	RPSTarget   float64
	CooldownMs    int64
}

func (p Policy) Validate() error {
	if p.MinReplicas < 1 {
		return fmt.Errorf("min_replicas must be >= 1")
	}
	if p.MaxReplicas < p.MinReplicas {
		return fmt.Errorf("max_replicas must be >= min_replicas")
	}
	if p.CPUTarget <= 0 && p.RPSTarget <= 0 {
		return fmt.Errorf("cpu_target or rps_target required")
	}
	if p.CooldownMs < 0 {
		return fmt.Errorf("cooldown_ms must be >= 0")
	}
	return nil
}

// DesiredReplicas computes next replica count from observed signals.
func DesiredReplicas(p Policy, current int32, cpuPercent, rps float64) int32 {
	_ = p.Validate()
	desired := current
	if p.CPUTarget > 0 && cpuPercent > p.CPUTarget {
		desired++
	}
	if p.RPSTarget > 0 && rps > p.RPSTarget {
		desired++
	}
	if p.CPUTarget > 0 && cpuPercent < p.CPUTarget*0.5 && desired > p.MinReplicas {
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
