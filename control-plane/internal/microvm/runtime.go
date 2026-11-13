package microvm

import (
	"fmt"
	"strings"
	"time"
)

// Hypervisor is the S11 guest runtime selection.
type Hypervisor string

const (
	HypervisorCloudHypervisor Hypervisor = "cloud-hypervisor"
	HypervisorFirecracker     Hypervisor = "firecracker"
)

// BootSpec describes a micro-VM launch request.
type BootSpec struct {
	WorkloadID string
	KernelPath string
	RootfsPath string
	VCPUs      uint32
	MemoryMB   uint32
	RequireKVM bool
	Hypervisor Hypervisor
}

// ValidateBootSpec enforces S11 readiness gates before launch.
func ValidateBootSpec(spec BootSpec) error {
	if strings.TrimSpace(spec.WorkloadID) == "" {
		return fmt.Errorf("workload_id is required")
	}
	if strings.TrimSpace(spec.KernelPath) == "" {
		return fmt.Errorf("kernel path is required")
	}
	if strings.TrimSpace(spec.RootfsPath) == "" {
		return fmt.Errorf("rootfs path is required")
	}
	if spec.VCPUs == 0 {
		return fmt.Errorf("vcpus must be >= 1")
	}
	if spec.MemoryMB < 128 {
		return fmt.Errorf("memory_mb must be >= 128")
	}
	if !spec.RequireKVM {
		return fmt.Errorf("microvm requires KVM")
	}
	switch spec.Hypervisor {
	case "", HypervisorCloudHypervisor, HypervisorFirecracker:
		return nil
	default:
		return fmt.Errorf("unsupported hypervisor %q", spec.Hypervisor)
	}
}

// SelectHypervisor picks CH by default (ADR spike); Firecracker if requested.
func SelectHypervisor(requested string) Hypervisor {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "firecracker", "fc":
		return HypervisorFirecracker
	default:
		return HypervisorCloudHypervisor
	}
}

// ColdBootBudget is the S11 acceptance target (< 500ms measured when hardware allows).
func ColdBootBudget() time.Duration {
	return 500 * time.Millisecond
}
