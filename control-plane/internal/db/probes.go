package db

import (
	"encoding/json"
	"strings"
)

// ProbeConfig is one liveness or readiness check (#17).
type ProbeConfig struct {
	Type                string   `json:"type"` // http | tcp | exec | none
	Path                string   `json:"path,omitempty"`
	Port                int32    `json:"port,omitempty"` // 0 = container_port
	Command             []string `json:"command,omitempty"`
	InitialDelaySeconds int32    `json:"initial_delay_seconds,omitempty"`
	PeriodSeconds       int32    `json:"period_seconds,omitempty"`
	TimeoutSeconds      int32    `json:"timeout_seconds,omitempty"`
	FailureThreshold    int32    `json:"failure_threshold,omitempty"`
}

// ProbeSpec holds readiness + liveness for a workload.
type ProbeSpec struct {
	Readiness ProbeConfig `json:"readiness"`
	Liveness  ProbeConfig `json:"liveness"`
}

func DefaultProbeSpec() ProbeSpec {
	return ProbeSpec{
		Readiness: ProbeConfig{Type: "http", Path: "/", PeriodSeconds: 10, TimeoutSeconds: 3, FailureThreshold: 3},
		Liveness:  ProbeConfig{Type: "none"},
	}
}

func NormalizeProbeSpec(spec ProbeSpec) ProbeSpec {
	def := DefaultProbeSpec()
	spec.Readiness = normalizeOneProbe(spec.Readiness, def.Readiness)
	spec.Liveness = normalizeOneProbe(spec.Liveness, ProbeConfig{Type: "none"})
	return spec
}

func normalizeOneProbe(p, fallback ProbeConfig) ProbeConfig {
	p.Type = strings.ToLower(strings.TrimSpace(p.Type))
	switch p.Type {
	case "http", "tcp", "exec":
		// ok
	case "", "none", "off":
		p.Type = "none"
	default:
		p = fallback
	}
	if p.Type == "none" {
		return ProbeConfig{Type: "none"}
	}
	if p.Type == "http" && strings.TrimSpace(p.Path) == "" {
		p.Path = "/"
	}
	if p.PeriodSeconds <= 0 {
		p.PeriodSeconds = 10
	}
	if p.TimeoutSeconds <= 0 {
		p.TimeoutSeconds = 3
	}
	if p.FailureThreshold <= 0 {
		p.FailureThreshold = 3
	}
	if p.InitialDelaySeconds < 0 {
		p.InitialDelaySeconds = 0
	}
	return p
}

func EncodeProbeSpec(spec ProbeSpec) ([]byte, error) {
	spec = NormalizeProbeSpec(spec)
	return json.Marshal(spec)
}

func DecodeProbeSpec(raw string) (ProbeSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return DefaultProbeSpec(), nil
	}
	var spec ProbeSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return DefaultProbeSpec(), err
	}
	return NormalizeProbeSpec(spec), nil
}

func ProbeSpecJSON(spec ProbeSpec) string {
	b, err := EncodeProbeSpec(spec)
	if err != nil {
		return "{}"
	}
	return string(b)
}
