# Sprint 12 — Snapshots, pools y API sandboxes

## Estado
**Cerrado 2026-11-13** (API/TTL/pool flags; snapshots de memoria reales quedan ligados a S11 hardware).

## Entregables
- CRUD SandboxService + UI; TTL obligatorio; pool_ready
- `SandboxExpired` GC helper + ops sandbox GC
- Target pool latency 50ms documentado

## Tests
`go test ./internal/db/ -run Sandbox -count=1`
