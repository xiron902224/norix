# Sprint 18 — Escalado y autoscaling

## Estado
**Cerrado 2026-11-13.**

## Entregables
- `autoscaler.Policy` + DesiredReplicas (CPU/RPS, cooldown, min/max)
- Replica_count already on workloads (S7)

## Tests
`go test ./internal/autoscaler/ -count=1`
