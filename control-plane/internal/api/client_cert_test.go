package api

import "testing"

func TestAgentMTLSRequired(t *testing.T) {
	t.Setenv("NORIX_REQUIRE_AGENT_MTLS", "")
	if AgentMTLSRequired() {
		t.Fatal("expected false by default")
	}
	t.Setenv("NORIX_REQUIRE_AGENT_MTLS", "1")
	if !AgentMTLSRequired() {
		t.Fatal("expected true for 1")
	}
}
