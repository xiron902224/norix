# Sprint 20 — Rendimiento, carga y caos

## Estado
**Cerrado 2026-11-13.**

## Entregables
- `test/load/k6-smoke.js` health load smoke
- `internal/chaos` scenario catalog for runbooks
- SLOs iniciales: healthz p99 < 100ms local; deploy success tracked in ops alerts

## Tests
`go test ./internal/chaos/ -count=1`
