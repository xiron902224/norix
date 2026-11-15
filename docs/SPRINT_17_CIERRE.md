# Sprint 17 — HA del control plane

## Estado
**Cerrado 2026-11-13** (leases Postgres; Patroni/multi-réplica documentado como deploy guide).

## Entregables
- Tabla `control_plane_leases` + `ha.AcquireLease` / `RenewLease`
- Default TTL 15s

## Tests
`go test ./internal/ha/ -count=1`
