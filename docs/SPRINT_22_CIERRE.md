# Sprint 22 — GA 1.0

## Estado
**Código cerrado 2026-11-15** (`VERSION` = 1.0.0).  
**Release pendiente:** no hay tag `v1.0.0` en el remoto hasta commit/CI/smoke.

## Entregables
- `VERSION` = 1.0.0
- `CHANGELOG.md`, `SECURITY.md`
- API freeze: `buf breaking` en CI
- Binarios multi-arch + `SHA256SUMS` + cosign keyless: `.github/workflows/release.yml`
- HA leases cableados, autoscaler apply, OIDC+TOTP real, DNS-01 lego, managed DB dumps/S3
- SDKs TS/Python/Go, status page, telemetría opt-in, Patroni compose, compose stacks, Wasm WASI HTTP

## Checklist producción

### Código / CI
- [x] Auth + RLS + TOTP + OIDC
- [x] Deploy imagen/git/compose
- [x] Edge TLS local + DNS-01 (env)
- [x] Ops backup/alertas + managed DB dump/S3
- [x] Scheduler multi-nodo
- [x] Observability
- [x] Docs upgrade/security
- [x] Workflow cosign + multi-arch Releases
- [x] HA leader leases en server
- [x] Autoscaler aplica réplicas
- [x] Status `/status` + telemetría opt-in

### Release / ops (abierto)
- [ ] Merge working tree → `develop` + CI verde
- [ ] Tag `v1.0.0` + assets en GitHub Releases
- [ ] Smoke E2E estable (deploy running + agent enroll)
- [ ] Validación host: WG apply / KVM micro-VM / DNS-01 provider / Patroni (según necesidad)

## Comunicación
v1.0 **código** incluye PaaS contenedores + identity + HA/HPA + supply-chain release workflow.  
Features gated por hardware/credenciales se activan por env. El anuncio GA público espera tag + smoke.
