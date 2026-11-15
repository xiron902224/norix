# Sprint 22 — candidatura GA → reclasificada a Beta 1

## Estado
**Código cerrado 2026-11-15** como superficie S22.  
**Release pública:** **`1.0.0-beta.1`** (no `v1.0.0` GA).  
Motivo: CI/gitflow/`main`/smoke ops incompletos para anunciar GA; ver `docs/ESTADO_Y_PLAN_V1.md`.

## Entregables
- `VERSION` = `1.0.0-beta.1`
- `CHANGELOG.md`, `SECURITY.md`, `LICENSE` (Apache-2.0), `CONTRIBUTING.md`
- API freeze relativo a `develop` vía `buf breaking` en CI
- Binarios multi-arch + `SHA256SUMS` + cosign keyless: `.github/workflows/release.yml`
- HA leases cableados, autoscaler apply, OIDC+TOTP real, DNS-01 lego, managed DB dumps/S3
- SDKs TS/Python/Go, status page, telemetría opt-in, Patroni compose, compose stacks, Wasm WASI HTTP

## Checklist

### Código / CI
- [x] Auth + RLS + TOTP + OIDC
- [x] Deploy imagen/git/compose
- [x] Edge TLS local + DNS-01 (env)
- [x] Ops backup/alertas + managed DB dump/S3
- [x] Scheduler multi-nodo
- [x] Observability
- [x] Docs upgrade/security + contrib + prod install
- [x] Workflow cosign + multi-arch Releases
- [x] HA leader leases en server
- [x] Autoscaler aplica réplicas
- [x] Status `/status` + telemetría opt-in
- [ ] CI verde en PR → `develop`

### Release / ops (abierto)
- [ ] Merge → `develop` + CI verde
- [ ] Tag `v1.0.0-beta.1` + assets en GitHub Releases
- [ ] Smoke E2E estable (deploy running + agent enroll)
- [ ] Validación host: WG apply / KVM micro-VM / DNS-01 provider / Patroni (según necesidad)
- [ ] GA `v1.0.0` solo tras checklist de `ESTADO` §4 “Must antes de GA”

## Comunicación
**Beta 1** incluye PaaS contenedores + identity + HA/HPA + supply-chain release workflow.  
Features gated por hardware/credenciales se activan por env. El anuncio **GA** espera smoke + `main` alineado + tag `v1.0.0`.
