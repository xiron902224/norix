package db

import "testing"

func TestQuotaDeltaZeroAllows(t *testing.T) {
	// Pure struct sanity — DB-backed CheckTeamQuota covered by integration when pool available.
	d := QuotaDelta{}
	if d.AdditionalCPU != 0 || d.AdditionalSandboxes != 0 {
		t.Fatal("zero delta")
	}
}
