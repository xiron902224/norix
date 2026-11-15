# Sprint 19 — Seguridad y cadena de suministro

## Estado
**Cerrado 2026-11-13.**

## Entregables
- `docs/threat-model-s19.md` STRIDE
- `scripts/sbom.sh` module SBOM
- Agent production checklist (existente)
- Rate limit + secrets key + RLS (base)

## Tests
`go test ./internal/secrets/ ./internal/api/ -count=1` (si hay tests secrets) + suite general
