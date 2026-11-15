package db

import "time"

// SandboxExpired reports whether a sandbox should be GC'd (S12 TTL).
func SandboxExpired(expiresAt time.Time, now time.Time) bool {
	if expiresAt.IsZero() {
		return false
	}
	return !expiresAt.After(now.UTC())
}

// SandboxPoolReadyLatencyMs is the S12 acceptance target from a warm pool (< 50ms).
func SandboxPoolReadyLatencyMs() int {
	return 50
}
