package db

import (
	"testing"
	"time"
)

func TestSandboxExpired(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	if !SandboxExpired(now.Add(-time.Second), now) {
		t.Fatal("past expiry")
	}
	if SandboxExpired(now.Add(time.Minute), now) {
		t.Fatal("future not expired")
	}
}

func TestSandboxPoolReadyLatencyMs(t *testing.T) {
	if SandboxPoolReadyLatencyMs() != 50 {
		t.Fatal("pool latency target")
	}
}
