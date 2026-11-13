package microvm

import "testing"

func TestValidateBootSpec(t *testing.T) {
	err := ValidateBootSpec(BootSpec{
		WorkloadID: "w1", KernelPath: "/boot/vmlinux", RootfsPath: "/var/lib/norix/rootfs.ext4",
		VCPUs: 1, MemoryMB: 512, RequireKVM: true, Hypervisor: HypervisorCloudHypervisor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ValidateBootSpec(BootSpec{WorkloadID: "w1", RequireKVM: false}) == nil {
		t.Fatal("expected kvm required")
	}
}

func TestSelectHypervisor(t *testing.T) {
	if SelectHypervisor("") != HypervisorCloudHypervisor {
		t.Fatal("default CH")
	}
	if SelectHypervisor("firecracker") != HypervisorFirecracker {
		t.Fatal("fc")
	}
}

func TestColdBootBudget(t *testing.T) {
	if ColdBootBudget().Milliseconds() != 500 {
		t.Fatal("budget")
	}
}
