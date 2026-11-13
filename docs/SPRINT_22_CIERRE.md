# Sprint 22 — GA 1.0

## Estado
**Cerrado 2026-11-13.** Tag objetivo: `v1.0.0` (`VERSION`).

## Entregables
- `VERSION` = 1.0.0
- `CHANGELOG.md`, `SECURITY.md`
- API freeze intent: buf breaking en CI
- Binarios: `make build` / install scripts + checksums path

## Checklist producción
- [x] Auth + RLS
- [x] Deploy imagen/git
- [x] Edge TLS local
- [x] Ops backup/alertas
- [x] Scheduler multi-nodo
- [x] Observability
- [x] Docs upgrade/security
- [ ] Firmas cosign en CI release (follow-up release engineering)
- [ ] Multi-arch release artifacts en GitHub Releases

## Comunicación
v1.0 incluye PaaS un/multi-nodo contenedores + edge + ops. Micro-VM/Wasm/HA Patroni quedan con paths implementados pero requieren hardware/ops adicionales para producción plena.
