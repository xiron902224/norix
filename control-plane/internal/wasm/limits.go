package wasm

import "fmt"

// Limits are S15 fuel/memory/time guards for wasm-worker.
type Limits struct {
	Fuel      uint64
	MemoryMB  uint32
	TimeoutMs uint32
}

// DefaultLimits returns conservative production defaults.
func DefaultLimits() Limits {
	return Limits{Fuel: 1_000_000, MemoryMB: 64, TimeoutMs: 5_000}
}

// Validate ensures limits stay within hard caps.
func (l Limits) Validate() error {
	if l.Fuel == 0 || l.Fuel > 50_000_000 {
		return fmt.Errorf("fuel out of range")
	}
	if l.MemoryMB == 0 || l.MemoryMB > 512 {
		return fmt.Errorf("memory_mb out of range")
	}
	if l.TimeoutMs < 10 || l.TimeoutMs > 60_000 {
		return fmt.Errorf("timeout_ms out of range")
	}
	return nil
}

// ColdStartBudgetMs is the S15 acceptance target (< 5ms when pooled).
func ColdStartBudgetMs() int {
	return 5
}
