package db

import "testing"

func TestDecodeProbeSpecDefaults(t *testing.T) {
	spec, err := DecodeProbeSpec("")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Readiness.Type != "http" || spec.Readiness.Path != "/" {
		t.Fatalf("readiness default: %+v", spec.Readiness)
	}
	if spec.Liveness.Type != "none" {
		t.Fatalf("liveness default: %+v", spec.Liveness)
	}
}

func TestNormalizeProbeSpecExec(t *testing.T) {
	spec := NormalizeProbeSpec(ProbeSpec{
		Readiness: ProbeConfig{Type: "exec", Command: []string{"true"}},
		Liveness:  ProbeConfig{Type: "tcp", Port: 8080},
	})
	if spec.Readiness.Type != "exec" || len(spec.Readiness.Command) != 1 {
		t.Fatalf("%+v", spec.Readiness)
	}
	if spec.Liveness.PeriodSeconds != 10 {
		t.Fatalf("period %d", spec.Liveness.PeriodSeconds)
	}
}
